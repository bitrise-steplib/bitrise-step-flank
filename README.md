# Flank

[![Step changelog](https://shields.io/github/v/release/bitrise-steplib/bitrise-step-flank?include_prereleases&label=changelog&color=blueviolet)](https://github.com/bitrise-steplib/bitrise-step-flank/releases)

Run your tests using Flank.

<details>
<summary>Description</summary>

Run your tests using Flank. The step will automatically detect which project type your flank config uses and the corresponding flank command will be ran.
</details>

## 🧩 Get started

Add this step directly to your workflow in the [Bitrise Workflow Editor](https://docs.bitrise.io/en/bitrise-ci/workflows-and-pipelines/steps/adding-steps-to-a-workflow.html).

You can also run this step directly with [Bitrise CLI](https://github.com/bitrise-io/bitrise).

## ⚙️ Configuration

<details>
<summary>Inputs</summary>

| Key | Description | Flags | Default |
| --- | --- | --- | --- |
| `google_service_account_json` | Service Account JSON key file content. | required, sensitive |  |
| `config_path` | Flank config file path. | required |  |
| `version` | Flank binary version. You can use any tag name that is available on https://github.com/Flank/flank/releases or latest which will download the latest non-pre-elease version. | required | `latest` |
| `command_flags` | These flags will be appended to the flank command. If your flank config is for Android projects then these flags will be appended after `flank android test` otherwise after `flank ios test`. |  |  |
</details>

<details>
<summary>Outputs</summary>
There are no outputs defined in this step
</details>

## 🙋 Contributing

We welcome [pull requests](https://github.com/bitrise-steplib/bitrise-step-flank/pulls) and [issues](https://github.com/bitrise-steplib/bitrise-step-flank/issues) against this repository.

For pull requests, work on your changes in a forked repository and use the Bitrise CLI to [run step tests locally](https://docs.bitrise.io/en/bitrise-ci/bitrise-cli/running-your-first-local-build-with-the-cli.html).

Learn more about developing steps:

- [Create your own step](https://docs.bitrise.io/en/bitrise-ci/workflows-and-pipelines/developing-your-own-bitrise-step/developing-a-new-step.html)
