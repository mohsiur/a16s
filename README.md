# a16s — Easily Manage AWS Resources in Terminal 🐱

`a16s` is a terminal application for browsing and operating AWS resources from a single k9s-style TUI. It started as a fork of [keidarcy/e1s](https://github.com/keidarcy/e1s) (ECS-only) and now also covers Lambda, SQS, and DynamoDB through a `:`-palette flow. Inspired by [k9s](https://github.com/derailed/k9s).

> **Status:** active fork. ECS feature parity with upstream is preserved; multi-service additions live behind the `:` palette.

## Quick start

```bash
go install github.com/mohsiur/a16s/cmd/a16s@latest
a16s
```

Open the palette with `:` and type a kind:

- `:cluster` — ECS clusters (default landing view)
- `:lambda` — Lambda functions
- `:sqs` — SQS queues
- `:ddb` (alias `:dynamodb`) — DynamoDB tables
- `:exit` / `:quit` / `:q` — leave the app

`Tab` cycles autocomplete (e.g. `:c<Tab>` rotates through `cluster`, `container`).

## AWS credentials and configuration

`a16s` uses the default [aws-cli configuration](https://github.com/aws/aws-cli/blob/develop/README.rst#configuration). It does not store or send your access key or secret key anywhere. Credentials are only used to securely connect to AWS APIs through the AWS SDK for Go.

You can choose AWS credentials and target region in three ways:

- Use your default AWS CLI profile and region.
- Override them at startup with `AWS_PROFILE`, `AWS_REGION`, `--profile`, or `--region`.
- Switch them while `a16s` is running with `Ctrl+P` for profiles and `Ctrl+R` for regions.

`a16s` reads local AWS shared config and credentials files, so it works with common setups such as static credentials, assume-role profiles, `credential_process`, and AWS IAM Identity Center or SSO-based configurations.

## Installation

`a16s` is available on Linux, macOS and Windows.

```bash
# go install (recommended while pre-release)
go install github.com/mohsiur/a16s/cmd/a16s@latest

# from source
git clone https://github.com/mohsiur/a16s.git
cd a16s
go build -o a16s ./cmd/a16s
```

Pre-built binaries, brew taps, and Docker images will be published once the fork stabilizes.

## Usage

Make sure you have the AWS CLI installed and properly configured with the necessary permissions, and the [Session Manager plugin](https://docs.aws.amazon.com/systems-manager/latest/userguide/session-manager-working-with-install-plugin.html) installed if you plan to use interactive exec or port forwarding features.

```
$ a16s -h
a16s is a terminal application to easily browse and manage AWS resources 🐱.
Check https://github.com/mohsiur/a16s for more details.

Usage:
  a16s [flags]

Flags:
      --cluster string       specify the default cluster
  -c, --config-file string   config file (default "$HOME/.config/a16s/config.yml")
  -d, --debug                sets debug mode
  -h, --help                 help for a16s
  -j, --json                 log output json format
  -l, --log-file string      specify the log file path (default "${TMPDIR}a16s.log")
      --profile string       specify the AWS profile
      --read-only            sets read only mode
  -r, --refresh int          specify the default refresh rate as an integer, sets -1 to stop auto refresh (sec) (default 30)
      --region string        specify the AWS region
      --service string       specify the default service (requires --cluster)
  -s, --shell string         specify interactive ecs exec shell (default "/bin/sh")
      --splash               display startup splash screen (AWS load runs before the UI) (default true)
      --theme string         specify color theme
  -v, --version              version for a16s
```

### Examples

```bash
# default config
$ a16s
# explicit profile + region
$ AWS_PROFILE=custom-profile AWS_REGION=us-east-2 a16s
$ a16s --profile custom-profile --region us-east-2
# default cluster + service
$ a16s --cluster cluster-1 --service service-1
# read-only, debug, no auto-refresh, custom log path, json, dracula theme
$ a16s --read-only --debug --refresh -1 --log-file /tmp/a16s.log --json --theme dracula
# disable startup splash
$ a16s --splash=false
```

### Config file

Default config file path is `$HOME/.config/a16s/config.yml`. You can specify a different config file with `--config-file`. Because `a16s` uses [viper](https://github.com/spf13/viper?tab=readme-ov-file#what-is-viper), standard config formats supported by viper can be used.

Typical settings:

- `theme`
- `refresh`
- `read-only`
- `log-file`
- default `cluster` and `service`
- `splash`
- color overrides

### Theme and colors

Theme and colors can be specified by options or config file. Full themes list can be found [here](https://github.com/keidarcy/alacritty-theme/tree/master/themes). If you prefer to use your own color theme, specify the colors in the config file.

```yml
colors:
  BgColor: "#272822"
  FgColor: "#f8f8f2"
  BorderColor: "#a1efe4"
  Black: "#272822"
  Red: "#f92672"
  Green: "#a6e22e"
  Yellow: "#f4bf75"
  Blue: "#66d9ef"
  Magenta: "#ae81ff"
  Cyan: "#a1efe4"
  Gray: "#808080"
```

### Key bindings

`a16s` supports Vim-style navigation: use `h`, `j`, `k`, `l` for left, down, up, right respectively.

Common shortcuts:

- `:` — open the kind palette (Tab cycles, Esc cancels).
- `?` — help page.
- `Ctrl+P` / `Ctrl+R` — switch AWS profile / region.
- `/` — table filter. `Esc` clears.
- `F1`–`F12` — sort the current table by column index.
- `d` — describe selected resource.
- `c` — copy current page name or describe content.
- `b` — open in AWS console.
- `r` — refresh.
- `s` — shell into supported task / instance / container.

Press `?` to see the full list.

### Development

```bash
go run cmd/a16s/main.go --debug --log-file /tmp/a16s.log
```

```bash
tail -f /tmp/a16s.log
```

## Features

### Multi-service browsing (fork additions)

- ECS, Lambda, SQS, and DynamoDB share the same `:` palette and table chrome.
- Background preload — the first `:lambda`, `:sqs`, or `:ddb` is instant after splash.
- Cross-kind navigation — Enter `:l` on a Lambda function with a DLQ to jump straight to the DLQ row in `:sqs`.
- Async build with a loading placeholder so the UI stays responsive on slow lists.

### ECS feature parity (from upstream e1s)

- Drill-down: clusters → services → tasks → containers.
- Describe clusters, services, deployments, revisions, tasks, containers, task defs, autoscaling.
- CloudWatch Logs (awslogs) and realtime log streaming for single-log-group cases.
- ECS Exec interactive shell into containers, plus instance shell via SSM.
- Update services, register new task definitions, stop tasks.
- Local + remote host port forwarding sessions.
- File transfer through S3-backed workflows.

### Navigation and discovery

- Vim-style navigation with rich keyboard shortcuts.
- In-table filtering with simple text or `column:value` syntax.
- Per-column sorting with function keys.
- Profile and region pickers with in-app switching.

### Customization

- Built-in themes plus per-color overrides in config.
- Configurable refresh interval, splash behavior, shell, default targets.

## Acknowledgements

`a16s` is a fork of [keidarcy/e1s](https://github.com/keidarcy/e1s). The ECS browsing experience and the broader k9s-inspired layout are upstream's work — credit goes to Xing Yahao and the e1s contributors. The multi-service `:` palette and the per-kind plumbing on top are this fork's additions.

## Thanks

- [tview](https://github.com/rivo/tview)
- [k9s](https://github.com/derailed/k9s)
- [keidarcy/e1s](https://github.com/keidarcy/e1s)
- [ecsview](https://github.com/swartzrock/ecsview)
