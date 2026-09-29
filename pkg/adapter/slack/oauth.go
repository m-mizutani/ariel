package slack

import (
	"context"
	"net/http"
	"strings"

	"github.com/slack-go/slack"

	"github.com/m-mizutani/ariel/pkg/domain/interfaces"
	"github.com/m-mizutani/ariel/pkg/domain/model"
)

// OAuth exchanges OAuth v2 authorization codes with oauth.v2.access.
type OAuth struct {
	clientID     string
	clientSecret string
	opts         []slack.OAuthOption
}

var _ interfaces.SlackOAuth = &OAuth{}

func NewOAuth(clientID, clientSecret string, opts ...slack.OAuthOption) *OAuth {
	return &OAuth{clientID: clientID, clientSecret: clientSecret, opts: opts}
}

func (o *OAuth) ExchangeCode(ctx context.Context, code, redirectURI string) (*model.SlackOAuthResult, error) {
	resp, err := slack.GetOAuthV2ResponseContext(ctx, http.DefaultClient, o.clientID, o.clientSecret, code, redirectURI, o.opts...)
	if err != nil {
		return nil, wrapError(err, "failed to exchange slack oauth code")
	}

	var scopes []string
	for s := range strings.SplitSeq(resp.AuthedUser.Scope, ",") {
		if s = strings.TrimSpace(s); s != "" {
			scopes = append(scopes, s)
		}
	}

	return &model.SlackOAuthResult{
		TeamID:      model.SlackTeamID(resp.Team.ID),
		UserID:      model.SlackUserID(resp.AuthedUser.ID),
		AccessToken: model.SlackUserToken(resp.AuthedUser.AccessToken),
		TokenType:   resp.AuthedUser.TokenType,
		Scopes:      scopes,
	}, nil
}
