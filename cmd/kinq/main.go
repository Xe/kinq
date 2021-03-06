package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Xe/kinq/internal/database"
	"github.com/Xe/kinq/internal/discord"
	"github.com/Xe/kinq/internal/ksecretbox"
	"github.com/Xe/kinq/internal/linkscraper"
	"github.com/asdine/storm/v2"
	"github.com/bwmarrin/discordgo"
	"github.com/caarlos0/env"
	"github.com/celrenheit/sandflake"
	_ "github.com/joho/godotenv/autoload"
	"github.com/kr/session"
	bolt "go.etcd.io/bbolt"
	"golang.org/x/oauth2"
	chi "gopkg.in/chi.v3"
	"gopkg.in/chi.v3/middleware"
	"within.website/ln"
	"within.website/ln/ex"
)

type config struct {
	Port                      string   `env:"PORT" envDefault:"9001"`
	SecretBoxKey              string   `env:"SECRET_BOX_KEY,required"`
	DBPath                    string   `env:"DB_PATH,required"`
	DiscordKey                string   `env:"DISCORD_KEY,required"`
	DiscordMonitorChannels    []string `env:"DISCORD_MONITOR_CHANNELS,required"`
	DiscordMustGuild          string   `env:"DISCORD_MUST_GUILD,required"`
	DiscordOAuth2ClientID     string   `env:"DISCORD_OAUTH2_CLIENT_ID,required"`
	DiscordOAuth2ClientSecret string   `env:"DISCORD_OAUTH2_CLIENT_SECRET,required"`
	DiscordOAuth2RedirectURL  string   `env:"DISCORD_OAUTH2_REDIRECT_URL,required"`

	E621APIKey       string `env:"E621_API_KEY,required"`
	DerpibooruAPIKey string `env:"DERPIBOORU_API_KEY,required"`
}

func requestIDMiddleware(next http.Handler) http.Handler {
	var g sandflake.Generator
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := g.Next().String()

		if rid := r.Header.Get("X-Request-Id"); rid != "" {
			id = rid + "," + id
		}

		ctx := ln.WithF(r.Context(), ln.F{
			"request_id": id,
		})
		r = r.WithContext(ctx)

		w.Header().Set("X-Request-Id", id)
		r.Header.Set("X-Request-Id", id)

		next.ServeHTTP(w, r)
	})
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

	bl := BoltLogger(db.Bolt, "ln", ln.NewTextFormatter())
	ln.AddFilter(bl)

	dg, err := discordgo.New("Bot " + cfg.DiscordKey)
	if err != nil {
		ln.FatalErr(ctx, err)
	}

	rs := &linkscraper.Rules{
		linkscraper.NewDerpiDirectScraper(cfg.DerpibooruAPIKey),
		linkscraper.NewDerpiCDNScraper(cfg.DerpibooruAPIKey),
	}

	i := database.NewStormImages(db, rs)

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
		Scopes:       []string{"email", "identify", "guilds"},
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

	r.Use(requestIDMiddleware)
	r.Use(middleware.RealIP)
	r.Use(ex.HTTPLog)

	r.Get("/", s.login)
	r.Get("/info", info)
	r.Get("/login", s.login)
	r.Get("/login/redirect", s.redirect)
	r.Get("/images/id/{id}/img", s.image)
	r.Get("/images/id/{id}/json", s.imageJSON)

	r.Route("/images", func(r chi.Router) {
		r.Use(s.isLoggedIn)

		r.Get("/", s.renderTemplatePage("index.html", nil).ServeHTTP)
		r.Get("/recent", s.recent)
		r.Get("/id/{id}", s.one)
		r.Get("/backup", s.backup)
		r.Get("/logs", bl.ServeHTTP)
	})

	mux := http.NewServeMux()
	mux.Handle("/static/", http.FileServer(http.Dir(".")))
	mux.Handle("/", r)

	ln.Log(ctx, ln.Action("serving http"), ln.F{"port": cfg.Port})
	http.ListenAndServe(":"+cfg.Port, mux)
}

type site struct {
	cfg    config
	oa2cfg *oauth2.Config
	scfg   *session.Config
	db     *storm.DB
	dg     *discordgo.Session
	i      database.Images
	g      sandflake.Generator
}

type sessionData struct {
	ID   string
	Code string
}

func info(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintln(w, `This is a private site for NSFW image archival.

If you see this link in your access logs, chances are that your site hosted
content we considered important enough to save. The code for this site is at
https://github.com/Xe/kinq. We are sorry if our honest attempt at _private_
archival bothers you. Please contact https://christine.website/contact if you
have any complaints or images you want removed.

Be well, Creator.`)
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

	found := false
	for _, id := range s.cfg.DiscordMonitorChannels {
		if id == mc.ChannelID {
			found = true
		}
	}

	if !found {
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
	u := s.oa2cfg.AuthCodeURL(s.g.Next().String())
	http.Redirect(w, r, u, http.StatusTemporaryRedirect)
}

func (s *site) redirect(w http.ResponseWriter, r *http.Request) {
	c := r.URL.Query().Get("code")

	session.Set(w, &sessionData{ID: s.g.Next().String(), Code: c}, s.scfg)
	http.Redirect(w, r, "/images", http.StatusTemporaryRedirect)
}

func (s *site) recent(w http.ResponseWriter, r *http.Request) {
	var pageID string
	keys, ok := r.URL.Query()["page"]
	if ok {
		pageID = keys[0]
	}
	i, _ := strconv.Atoi(pageID)
	is, err := s.i.Recent(i)
	if err != nil {
		ln.Error(r.Context(), err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	next := r.URL.Query()
	next.Set("page", strconv.Itoa(i+1))
	prev := r.URL.Query()
	prev.Set("page", strconv.Itoa(i-1))

	data := struct {
		Subtitle string
		Images   []database.Image
		NextURL  string
		PrevURL  string
	}{
		Subtitle: "recent images",
		Images:   is,
		NextURL:  r.URL.Path + "?" + next.Encode(),
		PrevURL:  r.URL.Path + "?" + prev.Encode(),
	}

	s.renderTemplatePage("imagelist.html", &data).ServeHTTP(w, r)
}

func (s *site) one(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	i, err := s.i.One(id)
	if err != nil {
		ln.Error(r.Context(), err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	s.renderTemplatePage("image.html", i).ServeHTTP(w, r)
}

func (s *site) image(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	i, err := s.i.One(id)
	if err != nil {
		ln.Error(r.Context(), err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if len(i.Data) == 0 {
		i, err = s.i.Insert(i.URL)
		if err != nil {
			ln.Error(r.Context(), err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	etag := "W/" + i.Blake2Hash

	if match := r.Header.Get("If-None-Match"); match != "" {
		if strings.Contains(match, etag) {
			w.WriteHeader(http.StatusNotModified)
			return
		}
	}

	w.Header().Set("Content-Type", i.Mime)
	w.Header().Set("Created-At", i.Added.Format(time.RFC3339))
	w.Header().Set("Image-Hash", i.Blake2Hash)
	w.Header().Set("ETag", etag)
	w.Write(i.Data)
}

func (s *site) imageJSON(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	i, err := s.i.One(id)
	if err != nil {
		ln.Error(r.Context(), err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(i)
}

func (s *site) backup(w http.ResponseWriter, r *http.Request) {
	err := s.db.Bolt.View(func(tx *bolt.Tx) error {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Disposition", `attachment; filename="kinq.db"`)
		w.Header().Set("Content-Length", strconv.Itoa(int(tx.Size())))
		_, err := tx.WriteTo(w)
		return err
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
