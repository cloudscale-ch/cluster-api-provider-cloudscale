# Contributing

Thanks for your interest in improving CAPCS.

## Issues

File bugs and feature requests in the
[GitHub issue tracker](https://github.com/cloudscale-ch/cluster-api-provider-cloudscale/issues).
Please include the CAPCS version, the Kubernetes version of your management
cluster, and the relevant CRD YAML when reporting a bug. If you are unsure
whether a problem is a bug, open an issue anyway — it is easier to redirect
than to discover later.

## Pull requests

1. Fork the repository and create a feature branch off `main`.
2. Make your change. Tests live next to the code; new behavior needs a test.
3. Run `make test` and `make lint` locally.
4. For changes that touch reconcilers or templates, run at least
   `make test-e2e-lifecycle` against a cloudscale.ch project.
5. Open a PR against `main`. Keep the title short and the description
   focused on the *why*.

Commit messages loosely follow
[Conventional Commits](https://www.conventionalcommits.org/) (`feat:`, `fix:`,
`chore:`, `docs:`). Match the style of recent commits.

## Development setup

See [docs/development.md](docs/development.md) for architecture, Tilt setup,
test layers, and make targets.

## Questions

Open an issue — there is no separate chat channel.
