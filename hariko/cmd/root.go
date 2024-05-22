package cmd

import (
	"bytes"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"

	"github.com/bwmarrin/discordgo"
	"github.com/go-playground/webhooks/v6/github"
	"github.com/spf13/cobra"
)

const (
	ColorSuccess = 0x3fb950
	ColorFailure = 0xf85149
)

func newCmd() *cobra.Command {
	var discordWebhookIDToken string
	var githubJobName string
	var githubRepository string
	var githubWebhookSecret string
	var namespace string
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
						cmd.PrintErrf("repository expected: %s, got: %s\n", githubRepository, payload.Repository.FullName)
						return
					}
					if payload.WorkflowJob.Name != githubJobName {
						cmd.PrintErrf("job name expected: %s, got: %s\n", githubJobName, payload.WorkflowJob.Name)
						return
					}
					if payload.WorkflowJob.Status != "completed" {
						cmd.PrintErrf("job stats expected: completed, got: %s\n", payload.WorkflowJob.Status)
						return
					}
					if payload.WorkflowJob.Conclusion != "success" {
						cmd.PrintErrf("job conclusion expected: success, got: %s\n", payload.WorkflowJob.Conclusion)
						return
					}
					st := discord(&discordgo.WebhookParams{
						Embeds: []*discordgo.MessageEmbed{
							{
								Title: "Deployment started",
							},
						},
					}, nil)
					b := new(bytes.Buffer)
					err := deploy(namespace, packageName, repositoryName, repositoryURL, b)
					if err != nil {
						discord(&discordgo.WebhookParams{
							Embeds: []*discordgo.MessageEmbed{
								{
									Title:       "Deployment failed",
									Color:       ColorFailure,
									Description: err.Error() + "\n```\n" + b.String() + "```",
								},
							},
						}, st)
						return
					}
					discord(&discordgo.WebhookParams{
						Embeds: []*discordgo.MessageEmbed{
							{
								Title:       "Deployment succeeded",
								Color:       ColorSuccess,
								Description: "```\n" + b.String() + "```",
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
	f.StringVarP(&namespace, "namespace", "n", "", "Namespace")
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

func deploy(namespace string, packageName string, repositoryName string, repositoryURL string, log io.Writer) error {
	if err := run(exec.Command("helm", "repo", "add", "-n", namespace, repositoryName, repositoryURL), log); err != nil {
		return err
	}
	if err := run(exec.Command("helm", "repo", "update"), log); err != nil {
		return err
	}
	if err := run(exec.Command("helm", "upgrade", "-n", namespace, packageName, repositoryName+"/"+packageName), log); err != nil {
		return err
	}
	return nil
}

func run(cmd *exec.Cmd, log io.Writer) error {
	cmd.Env = append(cmd.Env, "HELM_DRIVER=configmap")
	cmd.Stdout = log
	cmd.Stderr = log
	return cmd.Run()
}
