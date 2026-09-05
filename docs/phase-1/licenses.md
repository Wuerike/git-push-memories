# Licencas dos candidatos da Fase 1

Registro feito a partir dos arquivos presentes nas revisoes locais fixadas em 2026-09-04 e complementado pela triagem estatica de manifestos, subdiretorios, dependencias e ativos. Este documento orienta a avaliacao tecnica e nao substitui revisao juridica quando houver incorporacao de codigo ou distribuicao.

## Resumo

| Projeto | Revisao | Licenca raiz | Formas permitidas nesta fase | Obrigacoes ou restricoes principais |
|---|---|---|---|---|
| Basic Memory | `a55003e7` | AGPL-3.0-or-later | referencia e reimplementacao independente de conceitos | Nao fazer fork nem copiar codigo para o produto Apache-2.0 sem relicenciamento explicito. |
| ai-memory | `3bd35154` | MIT | fork, componente, reimplementacao ou referencia | Preservar aviso de copyright e texto da licenca em copias ou porcoes substanciais. |
| Palinode | `9832d586` | MIT | fork, componente, reimplementacao ou referencia | Preservar aviso de copyright e texto da licenca em copias ou porcoes substanciais. |
| claude-mem | `be44b6c8` | Apache-2.0 | fork, componente, reimplementacao ou referencia | Incluir licenca, marcar arquivos alterados, preservar avisos e reproduzir o `NOTICE` aplicavel. |
| claude-bedrock | `7cac16ce` | MIT | fork, componente, reimplementacao ou referencia | Preservar aviso de copyright e texto da licenca em copias ou porcoes substanciais. |
| agent-memory | `51dac9bf` | Apache-2.0 | fork, componente, reimplementacao ou referencia | Incluir licenca, marcar arquivos alterados e preservar avisos aplicaveis. |
| twiceshy | `aa8f1d88` | AGPL-3.0-only | referencia e reimplementacao independente de conceitos | Nao fazer fork nem copiar codigo para o produto Apache-2.0 sem relicenciamento explicito. |

As permissoes acima se referem somente ao codigo coberto pela licenca raiz. Dependencias, subdiretorios com licenca propria, modelos, datasets, marcas e outros ativos devem ser verificados antes de qualquer reutilizacao.

## Basic Memory

- Evidencia: `~/workspace/basic-memory/LICENSE` contem AGPL-3.0; `pyproject.toml` declara `AGPL-3.0-or-later`.
- Compatibilidade com a premissa: falha para fork ou componente incorporado a distribuicao Apache-2.0.
- Uso permitido no plano atual: executar localmente para avaliacao, citar comportamentos e reimplementar ideias sem copiar expressao protegida.
- Observacao decisiva: `plugins/claude-code/README.md` declara o plugin como MIT, mas o diretorio nao possui arquivo de licenca proprio na revisao avaliada. A origem e a extensao dessa declaracao devem ser esclarecidas antes de reutilizar qualquer arquivo do plugin.
- Escopo verificado na triagem: os hooks executaveis ficam em `src/basic_memory/cli/commands/hook.py`; nao ha aviso de licenca separado nesse caminho, portanto a afirmacao isolada de `plugins/claude-code/README.md` nao basta para separa-los da AGPL.
- Disposicao de licenca: referencia; o spike do plugin nao foi aberto e nenhuma reutilizacao direta esta na shortlist.

## ai-memory

- Evidencia: `~/workspace/ai-memory/LICENSE` contem MIT, copyright 2026 Fabio Akita.
- Compatibilidade com a premissa: parcial; a licenca raiz e permissiva, mas o escopo exato de uma eventual reutilizacao ainda nao foi auditado.
- Obrigacao para copia substancial: incluir o aviso de copyright e a permissao MIT.
- Dependencias e ativos: `deny.toml:16-42` configura `cargo-deny`, mas a politica nao foi executada nesta etapa; `crates/ai-memory-web/build.rs:20-69` e `crates/ai-memory-web/src/routes/statics.rs:9-14` mostram CSS/logo embutidos, e `companions/ai-memory-importer/Cargo.toml:1-25` nao declara licenca propria.
- Disposicao de licenca: reimplementacao conceitual; nenhum crate ou ativo entrou na lista de copia e a escolha de Go encerrou a auditoria adicional neste ciclo.

## Palinode

- Evidencia: `~/workspace/palinode/LICENSE` contem MIT, copyright 2026 Paul Kyle.
- Compatibilidade com a premissa: parcial para o produto completo; a fatia exata da Etapa 7 e permissiva, mas foi rejeitada pela ADR 0001.
- Obrigacao para copia substancial: incluir o aviso de copyright e a permissao MIT.
- Observacao: `claude-plugin/LICENSE` e `claude-plugin/.claude-plugin/plugin.json` confirmam MIT nesse subdiretorio. Dependencias Python/TypeScript, vetores identificados como CC BY 4.0 em `tests/test_amr_conformance.py:1-6` e avisos de codigo adaptado ainda devem ser inventariados antes de copia.
- Escopo verificado na Etapa 7: a fatia offline de parser, executor, hashing, quotes, claims e typed links foi extraida de arquivos cobertos pela MIT raiz. O runtime minimo usou `python-frontmatter==1.1.0` e `PyYAML==6.0.2`, ambos inventariados como MIT; seus textos e metadados estao na evidencia do spike.
- Exclusoes da fatia: plugin, prompts, vetores CC BY 4.0, modelos, ativos, API, store, indice, sessoes e integracoes nao foram aprovados para copia.
- Disposicao de licenca: referencia e reimplementacao conceitual. A ADR 0001 escolheu Go e nenhum fonte ou dependencia Palinode sera incorporado.

## claude-mem

- Evidencia: `~/workspace/claude-mem/LICENSE` contem Apache-2.0 e `~/workspace/claude-mem/NOTICE` identifica Claude-Mem, copyright 2026 Alex Newman.
- Compatibilidade com a premissa: parcial; a licenca raiz e permissiva, mas o escopo exato de uma eventual reutilizacao ainda nao foi auditado.
- Obrigacoes para redistribuicao: fornecer copia da Apache-2.0, marcar arquivos modificados, preservar avisos aplicaveis e reproduzir as atribuicoes aplicaveis do `NOTICE`.
- Excecao de escopo: `.claude-plugin/marketplace.json:9-21` inclui `claude-mem-cowork`, cujo `cowork/.claude-plugin/plugin.json:1-17` declara PolyForm-Noncommercial-1.0.0. Esse componente nao esta aprovado para incorporacao ao produto Apache-2.0.
- Ativos e dependencias: o clone nao possui inventario agregado para logos, fontes, slides, GIFs e bundle; `docs/license.md:26-29` tambem registra que Apache-2.0 nao concede direitos de marca.
- Disposicao de licenca: referencia e reimplementacao conceitual; nenhum arquivo entrou na shortlist. O subcomponente Cowork e os ativos permanecem excluidos.

## claude-bedrock

- Evidencia: `~/workspace/claude-bedrock/LICENSE` contem MIT, copyright 2026 Iury Krieger.
- Compatibilidade com a premissa: parcial; a licenca raiz e permissiva, mas o escopo exato de uma eventual reutilizacao ainda nao foi auditado.
- Obrigacao para copia substancial: incluir o aviso de copyright e a permissao MIT.
- Dependencias e ativos: `skills/setup/SKILL.md:116-191` instala graphify e docling sem pin ou inventario de licencas. Nao foi encontrada proveniencia individual para `docs/banner.png` ou `site/public/videos/`; esses ativos ficam fora de reutilizacao ate auditoria.
- Disposicao de licenca: reimplementacao conceitual e referencia; nenhum arquivo, ativo ou dependencia entrou na shortlist.

## agent-memory

- Evidencia: `~/workspace/agent-memory/LICENSE` contem Apache-2.0, copyright 2026 Teterichev Anton.
- Compatibilidade com a premissa: passa para os tres arquivos escolhidos; a licenca raiz Apache-2.0, os blobs, a unica dependencia externa e suas obrigacoes estao delimitados abaixo.
- Obrigacoes para redistribuicao: fornecer copia da Apache-2.0, marcar arquivos modificados e preservar avisos aplicaveis. Nenhum `NOTICE` raiz foi encontrado na revisao registrada.
- Dependencias e empacotamento: `go.mod:5-12` lista as dependencias diretas, mas o clone nao contem SBOM nem agregacao de notices. `npm/package.json:6-11` publica somente `bin/`, sem demonstrar que a licenca acompanha o tarball.
- Disposicao de licenca: a ADR 0001 aprovou a adaptacao de `parse.go`, `splice.go` e `section_id.go`. A Etapa 7 resolveu a viabilidade de roots, mas os demais pacotes `internal`, dependencias e correcoes do prototipo continuam fora da incorporacao.

## twiceshy

- Evidencia: `~/workspace/twiceshy/LICENSE` contem o texto AGPL-3.0 e `README.md` identifica a escolha como `AGPL-3.0-only`.
- Compatibilidade com a premissa: falha para fork ou componente incorporado a distribuicao Apache-2.0.
- Uso permitido no plano atual: executar localmente para avaliacao, citar comportamentos e reimplementar ideias sem copiar expressao protegida.
- Escopo adicional: `README.md:20-24` informa que o corpus canonico e mantido em repositorio separado; ele nao estava no clone, portanto licenca e ativos nao foram auditados nem sao candidatos a reutilizacao nesta fase.
- Disposicao de licenca: referencia para quarentena, validacao, promocao e governanca.

## Shortlist da Etapa 8 e decisao da Etapa 9

A [ADR 0001](../adr/0001-implementation-base.md) aprovou somente tres arquivos do agent-memory. Qualquer outro arquivo, teste, fixture, prompt, plugin ou ativo continua fora do escopo.

### agent-memory Markdown

Origem: `xChuCx/agent-memory@51dac9bf6e72c67f6314ee3abb00515986a3fa4a`, Apache-2.0, sem `NOTICE` raiz encontrado.

| Arquivo candidato | Git blob |
|---|---|
| `internal/markdown/parse.go` | `e4cde2d405d112a67b607f0e0f8a3747b81a76e3` |
| `internal/markdown/splice.go` | `36c81a926fbd97d89039ffa03d963a1ecc501c12` |
| `internal/markdown/section_id.go` | `aa801f90ce4ab852f7771000d9e0f81b648fbff6` |
| `internal/markdown/validate.go` | `ad50e61fa055b7c286b5d829a9c496d66ffbdd22` |
| `internal/markdown/merge.go` | `861d92bb230aab894c6ca2aacd424325f47432c4` |

`parse.go`, `splice.go` e `section_id.go` formam o conjunto aprovado de parsing e edicao byte-preserving. `parse.go` importa Goldmark; os outros dois usam somente a biblioteca padrao e simbolos definidos nesse conjunto. Goldmark 1.7.4 nao declara dependencias de modulo em seu `go.mod`. Assim, a closure externa de producao dessa fatia e somente `github.com/yuin/goldmark@v1.7.4`, MIT, copyright 2019 Yusuke Inuzuka. `validate.go` foi rejeitado porque nao valida o schema do produto; `merge.go` foi rejeitado porque merge entre branches esta fora da Fase 2 e sua politica nao foi adotada. Como o pacote esta sob `internal/`, a incorporacao copiara e renomeara a fonte, fornecera Apache-2.0, preservara avisos e marcara arquivos modificados.

`internal/memory`, staging, apply, scanner, Git, adapters e indice nao estao aprovados para copia. Sao referencias porque compartilham tipos internos, e a revisao fixa nao contem as correcoes de cold start, staging ID e drift demonstradas na Etapa 7. Os arquivos do prototipo foram produzidos durante a avaliacao e nao herdam automaticamente autoria ou licenca do upstream.

FTS5 tambem fica fora da shortlist inicial. Se a Fase 5 justificar o indice, sera necessario auditar `modernc.org/sqlite@v1.34.0` e sua closure, alem de `github.com/gofrs/flock@v0.12.1`, `gopkg.in/yaml.v3@v3.0.1` e todos os avisos efetivamente distribuidos.

### Palinode offline

Origem: `phasespace-labs/palinode@9832d58663077645e93093bac9208a084b1d7492`, MIT, sem `NOTICE` raiz encontrado.

A fatia tecnicamente demonstrada contem exatamente:

```text
LICENSE
palinode/__init__.py
palinode/core/__init__.py
palinode/core/parser.py
palinode/core/parity.py
palinode/core/hashing.py
palinode/core/quote_verify.py
palinode/core/claims.py
palinode/core/typed_links.py
palinode/consolidation/__init__.py
palinode/consolidation/op_parse.py
palinode/consolidation/executor.py
```

Os Git blobs e SHA-256 de cada arquivo estao preservados no [spike](spikes/palinode-offline-extraction.md) e em seu artefato. O runtime testado usou `python-frontmatter==1.1.0` e `PyYAML==6.0.2`, ambos MIT e sem outras dependencias instaladas. Distribuicao exige os avisos MIT independentes de Palinode, python-frontmatter e PyYAML.

A fatia nao e pacote publicado nem interface estavel. `executor.py` precisou de adapter para substituir `git_tools`; `typed_links.py` conserva import lazy para indexer; `json_repair` foi excluido; writes historicos nao sao transacao; `UPDATE` nao transporta evidencia. A ADR 0001 escolheu reimplementar os conceitos de evidencia em Go; a fatia nao sera incorporada.

### Registro esperado

| Item escolhido | Licenca | Material de distribuicao |
|---|---|---|
| tres fontes agent-memory selecionadas | Apache-2.0 | texto Apache, avisos preservados, arquivos modificados marcados |
| Goldmark 1.7.4 | MIT | copyright e texto MIT |

Palinode, python-frontmatter e PyYAML nao geram obrigacoes de distribuicao porque nenhuma fonte ou dependencia sera incorporada. Seus registros permanecem como evidencia da alternativa rejeitada.

## Precondicoes para incorporacao

O gate decisorio da Fase 1 esta fechado. Na mesma mudanca que incorporar o componente aprovado, registrar no inventario de terceiros e no material de distribuicao:

1. arquivo ou pacote exato e revisao de origem;
2. licenca aplicavel ao caminho, incluindo excecoes ou licencas de subdiretorios;
3. avisos e `NOTICE` que devem acompanhar a distribuicao;
4. dependencias transitivas e ativos nao cobertos pela licenca do codigo;
5. modificacoes realizadas e forma de atribuicao;
6. confirmacao de que a distribuicao final continua sob Apache-2.0.
