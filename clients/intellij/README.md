# AI Engineering OS — IntelliJ IDEA client

This directory contains the read-only IntelliJ IDEA client for AI Engineering OS.

The plugin is intentionally thin. The durable Go Control Plane remains the source of truth for coordination, Quality, recovery, memory, permissions and promotion.

## Actions

Available from the **Tools** menu:

- `AI Engineering OS: Start Control Plane`
- `AI Engineering OS: Open Control Plane`

The settings page under **Settings → Tools → AI Engineering OS** configures:

- AI Engineering OS binary path;
- loopback Control Plane address.

## Safety boundary

The configured address must be explicit loopback:

- `localhost:<port>`
- `127.x.x.x:<port>`
- `[::1]:<port>`

Starting the server uses IntelliJ's process API with an executable and explicit argument vector:

~~~text
ai run serve --addr 127.0.0.1:8765
~~~

Repository text is never concatenated into a shell command.

The plugin does not expose mutation or promotion operations.

## Build

The project uses IntelliJ Platform Gradle Plugin 2.20.0 and targets IntelliJ IDEA 2026.2.

~~~text
gradle test buildPlugin verifyPluginProjectConfiguration
~~~


## Installable package

Release CI builds a checksum-protected IntelliJ plugin distribution:

~~~text
ai-engineering-os_<product-version>_intellij.zip
ai-engineering-os_<product-version>_intellij.zip.sha256
~~~

Install the ZIP from IntelliJ IDEA using **Settings → Plugins → ⚙ → Install Plugin from Disk…**.

The plugin distribution version is injected from the release version without editing source files.
