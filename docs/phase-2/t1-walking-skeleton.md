# T1: protocolo executavel e walking skeleton

> Documento historico da interface validada na Fase 2. A interface atual usa configuracao global e nao possui `code-root`; consulte o README.

## Fixed point e seams

- Fixed point: `91c1a877f36f30e02e5ec9983a8277aaec95310f`.
- Configuracao: valida e canonicaliza `memory_root` e `code_root`; nao procura roots implicitamente.
- Aplicacao: interpreta comandos e mantem stdout exclusivo para respostas ou protocolo MCP.
- MCP: encapsula o SDK oficial e recebe configuracao ja validada.
- Protocolo: fornece envelopes JSON com `schema_version: 1`.

O walking skeleton nao registra tools. T4, T5 e T8 adicionarao tools ao mesmo servidor sem dar ao adapter acesso direto a Markdown, proposals ou Git.

## Toolchain

- Modulo: `github.com/Wuerike/git-push-memories`.
- Linguagem minima: Go 1.25.0.
- Toolchain fixado: Go 1.25.12 no `go.mod` e imposto por `GOTOOLCHAIN` no `Makefile`.
- SDK MCP: `github.com/modelcontextprotocol/go-sdk@v1.7.0`, commit `bc72835f62eb94d0fb484439f886b6885b075f36`.

Foi escolhida a ultima versao estavel disponivel do SDK oficial. Ela fornece servidor e cliente MCP, transporte stdio e suporte ao protocolo `2026-07-28`. A pre-release `v1.8.0-pre.2` nao entra no produto.

## Inventario MCP

O SDK MCP possui licenciamento misto Apache-2.0/MIT durante sua transicao de licenca. Documentacao upstream sob CC-BY-4.0 nao e incorporada. A closure abaixo e a closure externa ligada ao binario Linux segundo `go list -deps ./cmd/git-push-memories`; `go.sum` fixa os checksums.

| Modulo | Versao | Licenca |
|---|---|---|
| `github.com/modelcontextprotocol/go-sdk` | `v1.7.0` | Apache-2.0 e MIT |
| `github.com/google/jsonschema-go` | `v0.4.3` | MIT |
| `github.com/segmentio/asm` | `v1.1.3` | MIT |
| `github.com/segmentio/encoding` | `v0.5.4` | MIT |
| `github.com/yosida95/uritemplate/v3` | `v3.0.2` | BSD-3-Clause |
| `golang.org/x/oauth2` | `v0.35.0` | BSD-3-Clause |
| `golang.org/x/sync` | `v0.20.0` | BSD-3-Clause |
| `golang.org/x/sys` | `v0.41.0` | BSD-3-Clause |
| `golang.org/x/time` | `v0.15.0` | BSD-3-Clause |

Antes de distribuir binarios, o empacotamento deve incluir os textos e avisos exigidos por essas licencas. A incorporacao futura de Goldmark e dos tres fontes agent-memory tera inventario proprio em T2.

## Verificacao

| Comando | Cobertura |
|---|---|
| `make verify` | testes, race detector, vet e build com Go 1.25.12 |
| `make verify-offline` | mesma matriz com Go 1.25.12 e `GOPROXY=off` |

`help`, `version` e uma conexao MCP nao criam estado persistente. Roots ausentes, iguais, fora de Git ou abaixo da raiz da worktree falham antes da inicializacao do servidor.
