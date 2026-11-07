# AI Engineering OS — VS Code client

This directory contains the read-only VS Code client for AI Engineering OS.

The extension does **not** implement coordination, scheduling, mutation, Quality, memory, or promotion logic. The durable Control Plane remains the source of truth.

## Commands

- `AI Engineering OS: Start Control Plane`
- `AI Engineering OS: Open Control Plane`
- `AI Engineering OS: Show Runs`
- `AI Engineering OS: Refresh Status`

## Safety boundary

The configured Control Plane address must be explicit loopback:

- `localhost:<port>`
- `127.x.x.x:<port>`
- `[::1]:<port>`

The client only performs read-only HTTP requests and opens the embedded dashboard.

Starting the server uses VS Code's terminal API with the configured binary as the executable and explicit arguments:

~~~text
ai run serve --addr 127.0.0.1:8765
~~~

It does not construct a shell command from repository text.

## Development

No npm dependencies are required.

~~~text
cd clients/vscode
npm test
npm run check
~~~

To run in VS Code extension development mode, open this directory in VS Code and launch an Extension Development Host using the standard VS Code extension debugger configuration of your choice.


## Installable package

Release CI builds a checksum-protected VSIX:

~~~text
ai-engineering-os_<product-version>_vscode.vsix
ai-engineering-os_<product-version>_vscode.vsix.sha256
~~~

Install it from **Extensions → … → Install from VSIX…** or:

~~~sh
code --install-extension ai-engineering-os_<product-version>_vscode.vsix
~~~

For product release candidates, the VSIX is marked as a VS Code pre-release while keeping a numeric `major.minor.patch` extension manifest version.
