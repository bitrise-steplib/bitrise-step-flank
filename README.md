# Flank

> Run your tests using Flank. The step will automaticall detect which project type your flank config uses and the corresponding flank command will be ran.

## Inputs

- google_service_account_json: __(required)__ __(sensitive)__
    > Service Account JSON key file content.
- config_path: __(required)__
    > Flank config file path.
- version: latest __(required)__
    > Flank binary version. You can use any tag name that is available on https://github.com/TestArmada/flank/releases or latest which will download the latest non-pre-elease version.
- command_flags:
    > These flags will be appended to the flank command.

## Outputs

### Exported Environment variables

### Deployed Artifacts

- ./results/{latest-result-dir}/*: $BITRISE_DEPLOY_DIR/*

## Contribute

1. Fork this repository
1. Make changes
1. Submit a PR

## How to run this step from source

1. Clone this repository
1. `cd` to the cloned repository's root
1. Create a bitrise.yml (if not yet created)
1. Prepare a workflow that contains a step with the id: `path::./`
    > For example:
    > ```yaml
    > format_version: "6"
    > default_step_lib_source: https://github.com/bitrise-io/bitrise-steplib.git
    > 
    > workflows:
    >   my-workflow:
    >     steps:
    >     - path::./:
    >         inputs: 
    >         - google_service_account_json: $GOOGLE_SERVICE_ACCOUNT
    >         - config_path: ./flank.yml
    > ```
1. Run the workflow: `bitrise run my-workflow`

## About
This is an official Step managed by Bitrise.io and is available in the [Workflow Editor](https://www.bitrise.io/features/workflow-editor) and in our [Bitrise CLI](https://github.com/bitrise-io/bitrise) tool. If you seen something in this readme that never before please visit some of our knowledge base to read more about that:
  - devcenter.bitrise.io
  - discuss.bitrise.io
  - blog.bitrise.io