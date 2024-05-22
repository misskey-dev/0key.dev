package cmd

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
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
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/yaml"
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
			settings.SetNamespace(namespace)
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
					release, err := deploy(packageName, repositoryName, repositoryURL, b)
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

type RESTClientGetter struct {
	genericclioptions.RESTClientGetter
}

func (r RESTClientGetter) ToRESTConfig() (*rest.Config, error) {
	return rest.InClusterConfig()
}

func (r RESTClientGetter) ToDiscoveryClient() (discovery.CachedDiscoveryInterface, error) {
	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, err
	}
	client, err := discovery.NewDiscoveryClientForConfig(config)
	if err != nil {
		return nil, err
	}
	return memory.NewMemCacheClient(client), nil
}

func (r RESTClientGetter) ToRESTMapper() (meta.RESTMapper, error) {
	client, err := r.ToDiscoveryClient()
	if err != nil {
		return nil, err
	}
	resources, err := restmapper.GetAPIGroupResources(client)
	if err != nil {
		return nil, err
	}
	return restmapper.NewDiscoveryRESTMapper(resources), nil
}

func (r RESTClientGetter) ToRawKubeConfigLoader() clientcmd.ClientConfig {
	return &clientcmd.DefaultClientConfig
}

func deploy(packageName string, repositoryName string, repositoryURL string, log io.Writer) (*release.Release, error) {
	p := getter.All(settings)
	c := repo.Entry{
		Name: repositoryName,
		URL:  repositoryURL,
	}
	r, err := repo.NewChartRepository(&c, p)
	if err != nil {
		return nil, err
	}
	if settings.RepositoryCache != "" {
		r.CachePath = settings.RepositoryCache
	}
	index, err := r.DownloadIndexFile()
	if err != nil {
		return nil, err
	}
	b, _ := os.ReadFile(settings.RepositoryConfig)
	re := repo.NewFile()
	if b != nil {
		yaml.Unmarshal(b, &re)
	}
	re.Update(&c)
	err = os.MkdirAll(filepath.Dir(settings.RepositoryConfig), os.ModePerm)
	if err != nil && !os.IsExist(err) {
		return nil, err
	}
	if err := re.WriteFile(settings.RepositoryConfig, 0600); err != nil {
		return nil, err
	}
	if _, err := repo.LoadIndexFile(index); err != nil {
		return nil, err
	}
	actionConfig := new(action.Configuration)
	actionConfig.Init(RESTClientGetter{}, settings.Namespace(), "configmap", func(format string, v ...interface{}) {
		fmt.Fprintf(log, format+"\n", v...)
	})
	client := action.NewUpgrade(actionConfig)
	client.Namespace = settings.Namespace()
	chartPath, err := client.LocateChart(repositoryName+"/"+packageName, settings)
	if err != nil {
		return nil, err
	}
	ch, err := loader.Load(chartPath)
	if err != nil {
		return nil, err
	}
	return client.Run(repositoryName, ch, map[string]interface{}{})
}
