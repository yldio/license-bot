package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"text/tabwriter"
	"time"
	"path/filepath"

	"github.com/google/go-github/github"

	"gopkg.in/src-d/go-git.v4"
	"gopkg.in/src-d/go-git.v4/config"
	// "gopkg.in/src-d/go-git.v4/plumbing"
	"gopkg.in/src-d/go-git.v4/plumbing/object"
	"gopkg.in/src-d/go-git.v4/plumbing/transport/http"
	"gopkg.in/src-d/go-git.v4/storage/filesystem"

	"gopkg.in/src-d/go-billy.v3/memfs"

	homedir "github.com/mitchellh/go-homedir"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"golang.org/x/oauth2"
)

var cfgFile string
var org string
var license string
var user string
var accessToken string
var mpl = `// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

`
var extensions = map[string]string{".go": mpl, ".js": mpl}

// RootCmd represents the base command when called without any subcommands
var RootCmd = &cobra.Command{
	Use:   "license-bot",
	Short: "Trawl a Github Organisation for license conformance issues",
	Long: `Open-Source licenses sometimes have a multiple gotcha's when
open-sourcing. Such as requiring that all files that are under said
license have to have something in the header of the file.

This mini-bot will periodically scan/trawl your Github User/Organisation
for all repos, determine the license (or lack thereof) and submit a PR
to improve license conformance.
`,
	Run: func(cmd *cobra.Command, args []string) {
		ctx := context.Background()
		ts := oauth2.StaticTokenSource(
			&oauth2.Token{AccessToken: accessToken},
		)
		tc := oauth2.NewClient(ctx, ts)

		client := github.NewClient(tc)

		opt := &github.RepositoryListByOrgOptions{
			ListOptions: github.ListOptions{PerPage: 10},
		}
		githubLicense, _, _ := client.Licenses.Get(ctx, license)
		var allRepos []*github.Repository
		for {
			repos, resp, err := client.Repositories.ListByOrg(ctx, org, opt)
			if err != nil {
				fmt.Println(err)
				os.Exit(2)
			}
			allRepos = append(allRepos, repos...)
			if resp.NextPage == 0 {
				break
			}
			opt.Page = resp.NextPage
		}

		test := func(r *github.Repository) bool { return !*r.Private && !*r.Fork }
		var ret []*github.Repository
		for _, repo := range allRepos {
			if test(repo) {
				for _, b := range repo.Topics {
					if b == "open-source-candidate" {
						ret = append(ret, repo)
					}
				}
			}
		}

		w := tabwriter.NewWriter(os.Stdout, 2, 0, 5, ' ', 0)
		for _, repo := range ret {
			if repo.License != nil {
				fmt.Fprintf(w, "%s\t%s\n", *repo.Name, *repo.License.SPDXID)
			} else {
				fork, _, err := client.Repositories.CreateFork(ctx, org, *repo.Name, &github.RepositoryCreateForkOptions{})
				forked, _, err := client.Repositories.Get(ctx, user, *repo.Name)
				if err != nil {
					fmt.Println("Fork Error:", err, fork, forked)
				}
				cloneRepo(repo, forked, githubLicense)
				title := "I have some licenses for you to use!"
				head := fmt.Sprintf("%s:%s", user, "branch")
				base := "master"
				body := "Moar 🤖🤖🤖🤖"
				pr := &github.NewPullRequest{
					Title: &title,
					Head: &head,
					Base: &base,
					Body: &body,
				}
				_, _, _ = client.PullRequests.Create(ctx, org, *repo.Name, pr)
				fmt.Fprintf(w, "%s\t%s\n", *repo.Name, "No License")
			}
		}
		w.Flush()
	},
}

func PrintErr(err error) {
	if err != nil {
		fmt.Println(err)
	}
}

func cloneRepo(src *github.Repository, repo *github.Repository, githubLicense *github.License) {
	fs := memfs.New()
	dot, err := fs.Chroot(".git")
	PrintErr(err)
	s, err := filesystem.NewStorage(dot)

	// Clones the repository into the worktree (fs) and storer all the .git
	// content into the storer
	r, _ := git.Clone(s, fs, &git.CloneOptions{
		URL:      src.GetCloneURL(),
		Progress: os.Stdout,
	})

	w, err := r.Worktree()

	_, err = r.CreateRemote(&config.RemoteConfig{
		Name: "upstream",
		URLs: []string{repo.GetCloneURL()},
	})

	PrintErr(err)

	err = r.Fetch(&git.FetchOptions{
		RemoteName: "upstream",
		Progress: os.Stdout,
	})

	PrintErr(err)

	err = w.Checkout(&git.CheckoutOptions{
		Create: true,
		Branch: "refs/heads/branch",
	})

	PrintErr(err)

	ref, err := r.Head()
	PrintErr(err)
	// ... retrieving the commit object
	commit, err := r.CommitObject(ref.Hash())
	fmt.Println(commit)
	PrintErr(err)
	tree, err := commit.Tree()
	PrintErr(err)

	_, err = fs.Stat("LICENSE")
	if err == os.ErrNotExist {
		file, _ := fs.Create("LICENSE")
		io.WriteString(file, githubLicense.GetBody())
		file.Close()

		// ... get the files iterator and print the file
		tree.Files().ForEach(func(f *object.File) error {
			if val, ok := extensions[filepath.Ext(f.Name)]; ok {
				fmt.Printf("100644 blob %s    %s %s\n", f.Hash, f.Name, filepath.Ext(f.Name))
				fs.Rename(f.Name, ".tmp")
				input := []byte(val)
				file, err := fs.OpenFile(f.Name, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
				PrintErr(err)

				file.Write(input)
				tmp, err := fs.OpenFile(".tmp", os.O_RDWR, 0644)
				PrintErr(err)

				stat, err := fs.Stat(".tmp")
				PrintErr(err)

				b := make([]byte, stat.Size())
				_, err = tmp.ReadAt(b, 0)
				PrintErr(err)
				file.Write(b)
				file.Close()
				tmp.Close()
				// content, err := ioutil.ReadFile(f.Name)
				// PrintErr(err)
				// l := []byte(val)
				// full := append(l, content...)
				// fmt.Println(full)
				// ioutil.WriteFile(f.Name, full, 0644)
				PrintErr(err)
			}
			return nil
		})
	}

	status, err := w.Status()
	PrintErr(err)
	fmt.Println(status)

	_, err = w.Add("LICENSE")

	PrintErr(err)

	// Prints the content of the CHANGELOG file from the cloned repository
	// license, err := fs.Open("LICENSE")
	_, err = w.Commit("license: Adding MPL-2.0 License", &git.CommitOptions{
		All: true,
		Author: &object.Signature{
			Name:  "License Bot",
			Email: "tom+license-bot@yld.io",
			When:  time.Now(),
		},
	})

	err = r.Push(&git.PushOptions{
		RemoteName: "upstream",
		Auth:       http.NewBasicAuth(user, accessToken),
		RefSpecs:   []config.RefSpec{"+refs/heads/branch:refs/heads/branch"},
	})

	if err != nil {
		fmt.Println("Push Error:", err)
	}
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	if err := RootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(initConfig)

	// Here you will define your flags and configuration settings.
	// Cobra supports persistent flags, which, if defined here,
	// will be global for your application.
	RootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.license-bot.yaml)")
	RootCmd.PersistentFlags().StringVar(&accessToken, "accessToken", "", "Your Github Oauth 2.0 Access Token")
	viper.BindPFlag("accessToken", RootCmd.PersistentFlags().Lookup("accessToken"))
	RootCmd.PersistentFlags().StringVar(&org, "organisation", "", "Name of Github Organisation to search for repos (default is the authenticated Github user)")
	viper.BindPFlag("organisation", RootCmd.PersistentFlags().Lookup("organisation"))
	RootCmd.PersistentFlags().StringVar(&license, "license", "MPL-2.0", "Name of the license to conform to, if left blank it will be assumed")
	viper.BindPFlag("license", RootCmd.PersistentFlags().Lookup("license"))
	RootCmd.PersistentFlags().StringVar(&user, "user", "yld-license-bot", "The name of your lovely bot")
	viper.BindPFlag("user", RootCmd.PersistentFlags().Lookup("user"))
}

// initConfig reads in config file and ENV variables if set.
func initConfig() {
	if cfgFile != "" {
		// Use config file from the flag.
		viper.SetConfigFile(cfgFile)
	} else {
		// Find home directory.
		home, err := homedir.Dir()
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}

		// Search config in home directory with name ".license-bot" (without extension).
		viper.AddConfigPath(home)
		viper.SetConfigName(".license-bot")
	}

	viper.AutomaticEnv() // read in environment variables that match

	// If a config file is found, read it in.
	if err := viper.ReadInConfig(); err == nil {
		if accessToken == "" {
			accessToken = viper.GetString("accessToken")
		}
		if org == "" {
			org = viper.GetString("organisation")
		}
		fmt.Println("Using config file:", viper.ConfigFileUsed())
	}
}
