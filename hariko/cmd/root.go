package cmd

import (
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/bwmarrin/discordgo"
	"github.com/go-playground/webhooks/v6/github"
	"github.com/spf13/cobra"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/getter"
	"helm.sh/helm/v3/pkg/release"
	"helm.sh/helm/v3/pkg/repo"
)

const (
	ColorSuccess = 0x3fb950
	ColorFailure = 0xf85149
)

var settings = cli.New()

func newCmd() *cobra.Command {
	var discordWebhookIDToken string
	var githubJobName string
	var githubRepository string
	var githubWebhookSecret string
	var packageName string
	var repositoryName string
	var repositoryURL string
	var rootCmd = &cobra.Command{
		Use:   "hariko",
		Short: "CD bot for 0key.dev",
		Long:  "Hariko watches the GitHub repository and automatically deploys the application to the server.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			hook, err := github.New(github.Options.Secret(githubWebhookSecret))
			if err != nil {
				return err
			}
			discord := func(_ *discordgo.WebhookParams, _ *discordgo.Message) *discordgo.Message {
				return nil
			}
			if discordWebhookIDToken != "" {
				session, err := discordgo.New("")
				if err != nil {
					return err
				}
				p := strings.Split(discordWebhookIDToken, "/")
				discord = func(data *discordgo.WebhookParams, previous *discordgo.Message) *discordgo.Message {
					if previous != nil {
						st, err := session.WebhookMessageEdit(p[0], p[1], previous.ID, &discordgo.WebhookEdit{
							Content:         &data.Content,
							Components:      &data.Components,
							Embeds:          &data.Embeds,
							Files:           data.Files,
							Attachments:     &data.Attachments,
							AllowedMentions: data.AllowedMentions,
						})
						if err != nil {
							return nil
						}
						return st
					} else {
						st, err := session.WebhookExecute(p[0], p[1], true, data)
						if err != nil {
							return nil
						}
						return st
					}
				}
			}
			http.HandleFunc("/github", func(w http.ResponseWriter, r *http.Request) {
				payload, err := hook.Parse(r, github.WorkflowJobEvent)
				if err != nil {
					if err == github.ErrEventNotFound {
						return
					}
					cmd.PrintErrln(err.Error())
					return
				}
				switch payload := payload.(type) {
				case github.WorkflowJobPayload:
					if payload.Repository.FullName != githubRepository {
						cmd.PrintErrf("unsupported repository: %s\n", payload.Repository.FullName)
						return
					}
					if payload.WorkflowJob.Name != githubJobName {
						cmd.PrintErrf("unsupported job: %s\n", payload.WorkflowJob.Name)
						return
					}
					if payload.WorkflowJob.Status != "completed" {
						cmd.PrintErrf("unsupported status: %s\n", payload.WorkflowJob.Status)
						return
					}
					if payload.WorkflowJob.Conclusion != "success" {
						cmd.PrintErrf("unsupported conclusion: %s\n", payload.WorkflowJob.Conclusion)
						return
					}
					st := discord(&discordgo.WebhookParams{
						Embeds: []*discordgo.MessageEmbed{
							{
								Title: "Deployment started",
							},
						},
					}, nil)
					release, err := deploy(packageName, repositoryName, repositoryURL)
					if err != nil {
						discord(&discordgo.WebhookParams{
							Embeds: []*discordgo.MessageEmbed{
								{
									Title:       "Deployment failed",
									Color:       ColorFailure,
									Description: err.Error(),
								},
							},
						}, st)
						return
					}
					discord(&discordgo.WebhookParams{
						Embeds: []*discordgo.MessageEmbed{
							{
								Title: "Deployment succeeded",
								Color: ColorSuccess,
								Fields: []*discordgo.MessageEmbedField{
									{
										Name:   "Name",
										Value:  release.Name,
										Inline: true,
									},
									{
										Name:   "Namespace",
										Value:  release.Namespace,
										Inline: true,
									},
									{
										Name:   "Revision",
										Value:  strconv.Itoa(release.Version),
										Inline: true,
									},
								},
							},
						},
					}, st)
				default:
					cmd.PrintErrf("unsupported payload type: %T\n", payload)
				}
			})
			return http.ListenAndServe(":3000", nil)
		},
	}
	f := rootCmd.Flags()
	f.StringVarP(&discordWebhookIDToken, "discord-webhook-id-token", "w", "", "Discord webhook ID & token")
	f.StringVarP(&githubJobName, "github-job-name", "j", "", "Job name")
	f.StringVarP(&githubRepository, "github-repository", "g", "", "Repository")
	f.StringVarP(&githubWebhookSecret, "github-webhook-secret", "s", "", "GitHub webhook secret")
	f.StringVarP(&packageName, "package-name", "p", "", "Package name")
	f.StringVarP(&repositoryName, "repository-name", "r", "", "Repository name")
	f.StringVarP(&repositoryURL, "repository-url", "u", "", "Repository URL")
	rootCmd.MarkFlagRequired("github-job-name")
	rootCmd.MarkFlagRequired("github-repository")
	rootCmd.MarkFlagRequired("package-name")
	rootCmd.MarkFlagRequired("repository-name")
	rootCmd.MarkFlagRequired("repository-url")
	return rootCmd
}

func Execute() {
	err := newCmd().Execute()
	if err != nil {
		os.Exit(1)
	}
}

func deploy(packageName string, repositoryName string, repositoryURL string) (*release.Release, error) {
	p := getter.All(settings)
	actionConfig := new(action.Configuration)
	actionConfig.Init(settings.RESTClientGetter(), settings.Namespace(), "", func(_ string, _ ...interface{}) {})
	r, err := repo.NewChartRepository(&repo.Entry{
		Name: repositoryName,
		URL:  repositoryURL,
	}, p)
	if err != nil {
		return nil, err
	}
	index, err := r.DownloadIndexFile()
	if err != nil {
		return nil, err
	}
	if _, err := repo.LoadIndexFile(index); err != nil {
		return nil, err
	}
	client := action.NewUpgrade(actionConfig)
	chartPath, err := client.LocateChart(packageName, settings)
	if err != nil {
		return nil, err
	}
	ch, err := loader.Load(chartPath)
	if err != nil {
		return nil, err
	}
	return client.Run(repositoryName, ch, map[string]interface{}{})
}
