# Security policy

Uptimy Agent holds credentials (webhook URLs, bot tokens, database passwords, an Uptimy agent key), so we take reports seriously and respond quickly.

## Reporting a vulnerability

Please **don't open a public issue**. Report it privately through GitHub: on the [Security tab](https://github.com/uptimy/agent/security), choose **Report a vulnerability**. Include what you found, how to reproduce it, and the version (shown at the bottom of the sidebar, or `uptimy-agent --version`).

You'll hear back within 3 working days. We'll keep you updated while we fix it, credit you in the release notes if you'd like, and ask you to hold off on public disclosure until a fixed release is out.

## Supported versions

Security fixes go into the latest release. We don't backport to older versions, so please keep the agent up to date: images are published as `ghcr.io/uptimy/agent:<version>` and `:latest`, and binaries on the [releases page](https://github.com/uptimy/agent/releases).

## What's in scope

Anything that lets someone do more than their role allows, for example:

- reading credentials as a viewer, from the public status page, or from logs and error messages
- changing anything without being signed in as an admin, or through a cross-site request
- reading the agent's own settings through a monitor (e.g. `${ADMIN_PASSWORD}` in a target)
- using the agent's Uptimy key for more than managing its own heartbeat

Running your own checks against your own services, or an admin choosing to monitor an internal address, is how the agent is meant to work and isn't a vulnerability.

See the security model in [ARCHITECTURE.md](ARCHITECTURE.md#security-model) for what the agent is designed to protect.
