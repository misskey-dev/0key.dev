# 0key.dev

misskey-dev staging environment

## How to install

1. Setup PostgreSQL, Redis, and Meilisearch (optional) in your cluster
2. Fork this repository and configure .github/workflows/\*.yml and charts/\*/values.yaml to fit your environment
3. Create GitHub Webhook (subscribe `workflow_job`) and Discord Webhook
4. Configure Hariko

```bash
cat <<EOF | kubectl apply -f -
apiVersion: v1
kind: Secret
type: Opaque
data:
  discord-webhook-id-token:
  github-job-name:
  github-repository:
  github-webhook-secret:
  package-name:
  repository-name:
  repository-url:
EOF
```

5. Deploy Hariko to your cluster

```bash
helm repo add misskey-dev https://misskey-dev.github.io/0key.dev
helm update
helm install hariko misskey-dev/hariko
```

## How to update Misskey

1. Clone this repository to your local machine
2. Update misskey submodule to the commit you want to update to
3. Commit and push the changes

> [!IMPORTANT]
> Care should be taken to ensure that database migration is a forward-compatible change. If it is not forward compatible, you will need to stop the service before updating.

> [!WARNING]
> Automatic rollback with database rebasing is not supported. You need to manually rollback the database if you want to rollback the Misskey version.

## License

Licensed under either of

- Apache License, Version 2.0 ([LICENSE-APACHE](LICENSE-APACHE) or
  <http://www.apache.org/licenses/LICENSE-2.0>)
- MIT License ([LICENSE-MIT](LICENSE-MIT) or
  <http://opensource.org/licenses/MIT>)

at your option.

## Contribution

Unless you explicitly state otherwise, any contribution intentionally submitted
for inclusion in the work by you, as defined in the Apache-2.0 license, shall be
dual licensed as above, without any additional terms or conditions.
