# Third-party inventory

## agent-memory Markdown engine

- Origin: `xChuCx/agent-memory@51dac9bf6e72c67f6314ee3abb00515986a3fa4a`.
- License: Apache-2.0; copyright 2026 Teterichev Anton.
- License copy: [`agent-memory/LICENSE`](agent-memory/LICENSE).
- Upstream `NOTICE`: none at the recorded revision.
- Incorporated source: only `internal/markdown/parse.go`, `internal/markdown/splice.go`, and `internal/markdown/section_id.go`.
- Original blobs: `e4cde2d405d112a67b607f0e0f8a3747b81a76e3`, `36c81a926fbd97d89039ffa03d963a1ecc501c12`, and `aa801f90ce4ab852f7771000d9e0f81b648fbff6`, respectively.
- Modifications: strict canonical immediate anchors; duplicate-ID rejection; explicit heading syntax offsets including Setext; frontmatter masking for section identity; body-only replacement; removal of automatic anchor insertion; local names and diagnostics.

No upstream tests, fixtures, `validate.go`, `merge.go`, or other agent-memory source is incorporated.

## Goldmark

- Module: `github.com/yuin/goldmark@v1.7.4`.
- License: MIT; copyright 2019 Yusuke Inuzuka.
- License copy: [`goldmark/LICENSE`](goldmark/LICENSE).
- Production dependency closure introduced by the Markdown engine: Goldmark only; its `go.mod` declares no dependencies.

The project remains distributed under Apache-2.0. These notices preserve the independent terms and attribution of incorporated third-party material.
