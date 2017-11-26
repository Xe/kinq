package main

import (
	"context"
	"net/http"
	"strings"

	"github.com/Xe/kinq/internal/database"
	"github.com/Xe/kinq/internal/discord"
	"github.com/Xe/kinq/internal/ksecretbox"
	"github.com/Xe/ln"
	"github.com/Xe/uuid"
	"github.com/asdine/storm"
	"github.com/bwmarrin/discordgo"
	"github.com/caarlos0/env"
	_ "github.com/joho/godotenv/autoload"
	"github.com/kr/session"
	"golang.org/x/oauth2"
	chi "gopkg.in/chi.v3"
	"gopkg.in/chi.v3/middleware"
)

type config struct {
	Port                      string `env:"PORT" envDefault:"9001"`
	SecretBoxKey              string `env:"SECRET_BOX_KEY,required"`
	DBPath                    string `env:"DB_PATH,required"`
	DiscordKey                string `env:"DISCORD_KEY,required"`
	DiscordMonitorChannel     string `env:"DISCORD_MONITOR_CHANNEL,required"`
	DiscordOAuth2ClientID     string `env:"DISCORD_OAUTH2_CLIENT_ID,required"`
	DiscordOAuth2ClientSecret string `env:"DISCORD_OAUTH2_CLIENT_SECRET,required"`
	DiscordOAuth2RedirectURL  string `env:"DISCORD_OAUTH2_REDIRECT_URL,required"`
}

func main() {
	ctx := context.Background()
	var cfg config
	err := env.Parse(&cfg)
	if err != nil {
		ln.FatalErr(ctx, err)
	}

	db, err := storm.Open(cfg.DBPath)
	if err != nil {
		ln.FatalErr(ctx, err)
	}

	dg, err := discordgo.New("Bot " + cfg.DiscordKey)
	if err != nil {
		ln.FatalErr(ctx, err)
	}

	i := database.NewStormImages(db)

	skey, err := ksecretbox.ParseKey(cfg.SecretBoxKey)
	if err != nil {
		ln.FatalErr(ctx, err)
	}

	scfg := &session.Config{
		Name:     "kinq",
		HTTPOnly: true,
		Keys:     []*[32]byte{skey},
	}

	oa2cfg := &oauth2.Config{
		ClientID:     cfg.DiscordOAuth2ClientID,
		ClientSecret: cfg.DiscordOAuth2ClientSecret,
		Endpoint:     discord.Endpoint,
		RedirectURL:  cfg.DiscordOAuth2RedirectURL,
		Scopes:       []string{"email"},
	}

	s := &site{
		cfg:    cfg,
		oa2cfg: oa2cfg,
		scfg:   scfg,
		db:     db,
		dg:     dg,
		i:      i,
	}

	dg.AddHandler(s.messageCreate)

	err = dg.Open()
	if err != nil {
		ln.FatalErr(ctx, err)
	}

	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)

	r.Get("/login", s.login)
	r.Get("/login/redirect", s.redirect)

	ln.Log(ctx, ln.Action("serving http"), ln.F{"port": cfg.Port})
	http.ListenAndServe(":"+cfg.Port, r)
}

type site struct {
	cfg    config
	oa2cfg *oauth2.Config
	scfg   *session.Config
	db     *storm.DB
	dg     *discordgo.Session
	i      database.Images
}

type sessionData struct {
	ID string
}

func (s *site) isLoggedIn(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var ss sessionData
		err := session.Get(r, &ss, s.scfg)
		if err != nil {
			ln.Error(r.Context(), err, ln.Action("redirecting to /login"))
			http.Redirect(w, r, "/login", http.StatusTemporaryRedirect)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func (s *site) messageCreate(ds *discordgo.Session, mc *discordgo.MessageCreate) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if mc.ChannelID != s.cfg.DiscordMonitorChannel {
		return
	}

	for _, word := range strings.Fields(mc.Content) {
		if !strings.HasPrefix(word, "http") {
			continue
		}

		i, err := s.i.Insert(word)
		if err != nil {
			ln.Error(ctx, err, ln.Action("saving attachments for message"), ln.F{"message_id": mc.ID, "author": mc.Author.Username, "url": word})
			continue
		}

		ln.Log(ctx, i, ln.Action("saved image"))

		ds.MessageReactionAdd(mc.ChannelID, mc.ID, "💾")
	}

	for _, att := range mc.Attachments {
		i, err := s.i.Insert(att.URL)
		if err != nil {
			ln.Error(ctx, err, ln.Action("saving attachments for message"), ln.F{"message_id": mc.ID, "author": mc.Author.Username})
			continue
		}

		ln.Log(ctx, i, ln.Action("saved image"))

		ds.MessageReactionAdd(mc.ChannelID, mc.ID, "💾")
	}
}

func (s *site) login(w http.ResponseWriter, r *http.Request) {
	u := s.oa2cfg.AuthCodeURL(uuid.New())
	http.Redirect(w, r, u, http.StatusTemporaryRedirect)
}

func (s *site) redirect(w http.ResponseWriter, r *http.Request) {
	session.Set(w, &sessionData{ID: uuid.New()}, s.scfg)
	http.Redirect(w, r, "/images", http.StatusTemporaryRedirect)
}
