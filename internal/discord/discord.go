// Package discord provides constants for using OAuth2 to access the Discord API.
package discord

import (
	"golang.org/x/oauth2"
)

// Endpoint is the Discord API's OAuth 2.0 endpoint.
var Endpoint = oauth2.Endpoint{
	AuthURL:  "https://discordapp.com/api/oauth2/authorize",
	TokenURL: "https://discordapp.com/api/oauth2/token",
}
