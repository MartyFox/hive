# Third-Party Notices

Hive is licensed under the Apache License, Version 2.0. This file summarizes
third-party software referenced by the source tree and Go binary builds.

This file is informational and may be incomplete for container image
redistribution. Published container images also include operating system
packages, Node.js, GitHub CLI, and agent CLI packages installed at image build
time. Redistributors of container images should preserve the notices and license
metadata provided by those upstream packages.

## Go Dependencies

| Module | Version | License |
|---|---:|---|
| `github.com/spf13/cobra` | `v1.10.2` | Apache-2.0 |
| `github.com/spf13/pflag` | `v1.0.9` | BSD-3-Clause |
| `github.com/inconshreveable/mousetrap` | `v1.1.0` | Apache-2.0 |
| `gopkg.in/yaml.v3` | `v3.0.1` | MIT and Apache-2.0 |

## Runtime Image Components

Hive Containerfiles install or rely on these notable upstream components:

| Component | Source / package |
|---|---|
| Debian base image | `node:22-bookworm-slim` |
| GitHub CLI | Debian package `gh` from GitHub CLI apt repository |
| Claude Code | npm package `@anthropic-ai/claude-code` |
| GitHub Copilot CLI | npm package `@github/copilot` |
| Google Gemini CLI | npm package `@google/gemini-cli` |
| OpenAI Codex CLI | npm package `@openai/codex` |
| Beads, optional | npm package `@beads/bd` |

## BSD-3-Clause Notice: github.com/spf13/pflag

Copyright (c) 2012 Alex Ogier. All rights reserved.
Copyright (c) 2012 The Go Authors. All rights reserved.

Redistribution and use in source and binary forms, with or without
modification, are permitted provided that the following conditions are met:

- Redistributions of source code must retain the above copyright notice, this
  list of conditions and the following disclaimer.
- Redistributions in binary form must reproduce the above copyright notice,
  this list of conditions and the following disclaimer in the documentation
  and/or other materials provided with the distribution.
- Neither the name of Google Inc. nor the names of its contributors may be used
  to endorse or promote products derived from this software without specific
  prior written permission.

THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS "AS IS" AND
ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE IMPLIED
WARRANTIES OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE ARE
DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT OWNER OR CONTRIBUTORS BE LIABLE FOR
ANY DIRECT, INDIRECT, INCIDENTAL, SPECIAL, EXEMPLARY, OR CONSEQUENTIAL DAMAGES
(INCLUDING, BUT NOT LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR SERVICES;
LOSS OF USE, DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER CAUSED AND ON
ANY THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY, OR TORT
(INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE OF THIS
SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.

## MIT Notice: gopkg.in/yaml.v3

The following files in `gopkg.in/yaml.v3` were ported to Go from C files of
libyaml and are covered by their original MIT license:

`apic.go`, `emitterc.go`, `parserc.go`, `readerc.go`, `scannerc.go`,
`writerc.go`, `yamlh.go`, `yamlprivateh.go`.

Copyright (c) 2006-2010 Kirill Simonov
Copyright (c) 2006-2011 Kirill Simonov

Permission is hereby granted, free of charge, to any person obtaining a copy of
this software and associated documentation files (the "Software"), to deal in
the Software without restriction, including without limitation the rights to use,
copy, modify, merge, publish, distribute, sublicense, and/or sell copies of the
Software, and to permit persons to whom the Software is furnished to do so,
subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY, FITNESS
FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR
COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER
IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN
CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.

## Apache-2.0 Notice: gopkg.in/yaml.v3

Copyright 2011-2016 Canonical Ltd.

Licensed under the Apache License, Version 2.0. See `LICENSE` for the full
license text.
