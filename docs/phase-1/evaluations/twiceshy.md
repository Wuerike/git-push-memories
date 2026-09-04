# Avaliacao estatica de twiceshy

## Identificacao

| Campo | Valor |
|---|---|
| Projeto | twiceshy |
| Repositorio | `https://github.com/dotts-h/twiceshy.git` |
| Clone local | `~/workspace/twiceshy` |
| Revisao | `aa8f1d88b23ce26b9add249ee2f507d350aca12c` |
| Branch | `main`, alinhada a `origin/main` |
| Estado inicial | arvore limpa |
| Data | 2026-09-04 |
| Avaliacao | triagem estatica, sem execucao do produto ou dos testes |
| Avaliador | OpenCode |
| Run ID | nao se aplica; nenhuma execucao comportamental |
| Commit inicial da memoria | nao se aplica; repositorios descartaveis nao foram criados |

## Hipotese

twiceshy sera apenas referencia por causa da AGPL, mas pode informar quarentena, validacao, promocao, feedback e governanca por PR.

## Escopo e metodo

Foram inspecionados CLI, servidor, ingestao, screen, schema, indice, spool, promocao, scripts Git, hooks e testes. Nao foram executados Go, daemon, Docker/runsc, judge, hooks ou testes.

```bash
git rev-parse HEAD
git branch --show-current
git remote get-url origin
git status --short --branch
git log -1 --format='%cI %s'
git rev-list --count HEAD
git rev-list --count --since='2026-08-05T00:00:00Z' HEAD
git shortlog -sne HEAD
```

Os comandos retornaram `0`; a duracao nao foi medida. O projeto usa Go 1.25.12, SQLite/FTS5 puro Go, MCP, YAML e JSON Schema, com Docker/runsc para repros (`go.mod:1-10`; `internal/repro/broker.go:52-67,185-229`). O historico local tem 967 commits, nenhum desde 2026-08-05 e 966 atribuidos a identidades Claude; a revisao e de 2026-07-11.

Ambiente usado na triagem: Linux x86_64 e Git 2.54.0, conforme [`evaluation-protocol.md`](../evaluation-protocol.md#ambiente-de-referencia). Somente Git e leitura estatica de arquivos foram usados.

## Fluxo estatico

| Estagio | Evidencia no clone | Conclusao |
|---|---|---|
| Entrada | `cmd/twiceshy/learned.go:29-180`; `internal/server/record.go:35-355` | CLI/MCP recebem registro estruturado e podem escrever ou enfileirar draft. |
| Quarentena | `internal/ingest/prepare.go:64-159` | Dedup exato/similar, proveniencia e status `quarantined`. |
| Filtro | `internal/screen/screen.go:31-138`; `internal/server/record.go:175-197` | Default sinaliza segredo/PII sem remover; postura fail-closed e restrita a alpha tenants. |
| Persistencia | `internal/record/marshal.go:11-28`; `cmd/twiceshy/main.go:667-708` | Um registro Markdown por arquivo, escrito atomicamente. |
| Promocao | `internal/promote/promote.go:140-284`; `internal/judge/judge.go:38-105` | Repro/attestation/judge podem promover a validated. |
| Publicacao | `scripts/scheduled-import.sh:110-191`; `scripts/scheduled-validate.sh:211-353` | Scripts criam branch/PR e podem auto-mergear apos janela de veto. |
| Recuperacao | `internal/index/index.go:808-933`; `internal/server/server.go:403-589` | Fingerprint, BM25 e vetor opcional retornam poucos registros validados. |

## Armazenamento e dependencias

O corpus Git separado de `experience/**/*.md` e a memoria canonica (`docs/SCHEMA.md:20-31`). O corpus real vive em outro repositorio e nao foi inspecionado (`README.md:20-24`). SQLite guarda FTS/fingerprints derivados, mas tambem tenant registry, quotas e uso nao reconstruiveis (`internal/index/index.go:303-509`; `docs/adr/ADR-0034-tenant-registry-is-not-derived-state.md:10-52`). A parte de memoria pode ser reconstruida; o arquivo inteiro nao e descartavel.

O fluxo Claude/MCP exige `twiceshy serve`, corpus, SQLite e token. Vetores/Ollama sao opcionais. Promocao completa exige Docker, runsc, imagem fixada e judge HTTP; producao rejeita familias locais de judge (`internal/judge/model.go:54-65,128-163`).

## Privacidade, sessoes e estado atual

No fluxo padrao, achados apenas viram `security_flags`; o texto bruto permanece. Filas podem persistir esse conteudo. Hooks Claude enviam prompt, erro e ate 200 kB de transcript (`hooks/twiceshy-push.sh:14-48`; `hooks/twiceshy-error-pull.sh:30-71`; `hooks/session-retro.sh:20-52`). Spool guarda session ID e transcript (`internal/spool/spool.go:20-30,104-126,203-224`).

O schema possui `superseded`, validade e `superseded_by`, mas nao existe fluxo generico que transforme uma descoberta em UPDATE/SUPERSEDE; esse doctor foi adiado (`docs/adr/ADR-0010-doctors-build-d2-defer-the-rest.md:27-31,46-54`). Um registro similar vira outro quarantined, nao substitui o anterior.

## Revisao, publicacao e Claude Code

Draft, spool, quarantine e PR separam captura de promocao. Entretanto `learned`/ingest podem escrever Markdown diretamente. Branch protection versionada exige zero aprovacoes (`branch-protection.json:2-10`); os scripts auto-mergeiam apos janela de veto, e existe atalho de drain (`scripts/twiceshy-drain-merge.sh:1-31`). Isso nao satisfaz aceite humano explicito.

Claude Code usa MCP e hooks UserPromptSubmit, PostToolUse e SessionEnd (`hooks/README.md:7-57`). Instalacao e manual e tudo depende do servidor. O ciclo nao foi executado.

## Testes encontrados

Ha muitos testes de schema, index/rebuild, dedup/quarantine, spool, PII, retro, promocao e tenant registry (`internal/record/schema_test.go:70-121`; `internal/index/index_test.go:85-315`; `internal/ingest/prepare_test.go:94-239`; `internal/server/record_redact_test.go:15-149`; `internal/promote/promote_test.go:140-259`). O sandbox real e opt-in e normalmente skipped (`internal/repro/broker_integration_test.go:17-24`). O corpus real e Claude Code nao sao cobertos. Nada foi executado.

## Criterios

| Criterio | Classificacao | Evidencia | Justificativa estatica |
|---|---|---|---|
| Markdown e Git canonicos | parcial | [Armazenamento](#armazenamento-e-dependencias) | O corpus externo nao estava no clone para inspecao. |
| Operacao local | pendente | [Dependencias](#armazenamento-e-dependencias) | Recuperacao pode ser local; falta fluxo offline. |
| Ausencia de backend obrigatorio | falha | [Dependencias](#armazenamento-e-dependencias) | MCP/hooks exigem servidor ativo. |
| Indice reconstruivel | pendente | [Fluxo](#fluxo-estatico) | Rebuild existe; equivalencia do corpus nao foi demonstrada. |
| Privacidade antes da persistencia | falha | [Privacidade](#privacidade-sessoes-e-estado-atual) | Default preserva segredo/PII bruto e apenas marca. |
| Sem persistencia de sessoes | falha | [Sessoes](#privacidade-sessoes-e-estado-atual) | IDs e transcripts sao persistidos em spool/telemetria. |
| Estado atual | falha | [Estado atual](#privacidade-sessoes-e-estado-atual) | Nao ha reconciliacao generica; similar gera outro registro. |
| Consolidacao antes da publicacao | parcial | [Revisao](#revisao-publicacao-e-claude-code) | Draft/quarantine/PR existem, sem consolidacao semantica. |
| Revisao explicita | parcial | [Revisao](#revisao-publicacao-e-claude-code) | PR permite veto, sem aprovacao obrigatoria e com auto-merge. |
| Repositorio de memoria separado | passa | [Armazenamento](#armazenamento-e-dependencias) | Corpus externo e desacoplado do engine. |
| Integracao com Claude Code | pendente | [Claude Code](#revisao-publicacao-e-claude-code) | MCP/hooks existem; falta execucao isolada. |
| Testabilidade | pendente | [Testes](#testes-encontrados) | Evidencia estatica forte; toolchain/integracoes nao foram executados. |
| Licenca compativel | falha | [Licencas](../licenses.md#twiceshy) | AGPL impede fork/componente Apache-2.0; referencia continua permitida. |
| Custo de adaptacao | passa | [Custo](#riscos-custo-e-duvidas) | Fork e inviavel; reimplementacao seletiva tem custo medio. |

## Riscos, custo e duvidas

A licenca elimina a adocao direta e nenhuma execucao pode mudar isso. Como referencia, os maiores riscos sao daemon, transcript, politica default de segredo bruto, falta de estado atual e auto-merge sem aceite. Reimplementar de forma independente somente o contrato de quarantine, estados de confianca e recuperacao precision-first tem custo medio; reproduzir a promocao com repro/judge teria custo alto e nao e necessario para a decisao atual.

Fluxo restante limitado: inspecionar o repositorio externo do corpus apenas se o formato/schema de registros for selecionado para reimplementacao conceitual. Os demais comportamentos nao precisam ser executados para decidir a disposicao.

## Recomendacao preliminar

**Referencia.** Permanecem relevantes o contrato uniforme de draft/quarantine, proveniencia tipada, estados de confianca, dedup em camadas, attestation/judge, counter-evidence, PR como janela de veto, recuperacao precision-first e envelope de dados nao instrucionais.
