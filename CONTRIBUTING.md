# Contributing to TrustIssues

Run the checks listed in README.md before submitting a change. Use temporary data directories for experiments. Installation data, private documents, credentials and network identities belong outside version control.

Keep pull requests focused on one behavior. Explain the user impact and validation. The root Go module contains the app and enrollment tests. Fabric chaincode and gateway are separate modules with their own dependencies.

For a release, update VERSION and docs/RELEASE_NOTES.md, push a matching v-prefixed tag, inspect the draft release assets and checksums, and publish the draft when ready.
