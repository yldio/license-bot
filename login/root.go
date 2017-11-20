package login

import (
	"context"

	"github.com/spf13/viper"
	"github.com/google/go-github/github"
	"golang.org/x/oauth2"
)

func login() *github.Client {
		accessToken := viper.GetString("accessToken")
		ctx := context.Background()
		ts := oauth2.StaticTokenSource(
			&oauth2.Token{AccessToken:accessToken},
		)
		tc := oauth2.NewClient(ctx, ts)

		client := github.NewClient(tc)
		return client
}
