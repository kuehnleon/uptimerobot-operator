# Contributing

Thanks for helping improve the UptimeRobot Operator. Issues and pull requests are welcome.

## Before opening an issue

- Search existing issues for the same problem or request.
- Include the operator, Kubernetes, and Helm versions where relevant.
- Redact API keys, credentials, and sensitive monitor URLs from logs and examples. Use [private vulnerability reporting](https://github.com/kuehnleon/uptimerobot-operator/security/advisories/new) for security issues.

## Development

Use the Go version declared in `go.mod`. The main checks are:

```sh
make generate   # regenerate deepcopy code and CRDs after API changes
make test       # go vet and unit/envtest tests
make helm-lint  # lint and render the chart
make helm-docs  # regenerate the chart README from values.yaml
```

Run `make generate` and `make helm-docs` after changing API types or chart values, then include the generated files in your pull request. CI checks generated output, tests, lint, and image builds.

## Pull requests

- Keep changes focused and include tests or documentation where appropriate.
- Pull requests are squash merged. Use a [Conventional Commit](https://www.conventionalcommits.org/) title, for example `fix(controller): retry failed deletes` or `feat(monitor): support regions`.
- Do not edit `CHANGELOG.md` manually; it is generated during releases.
- Never commit API keys, kubeconfigs, or other secrets.
