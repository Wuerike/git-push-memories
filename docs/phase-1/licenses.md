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
- Disposicao de licenca: referencia; eventual reutilizacao do plugin depende do spike de esclarecimento ja previsto no plano.

## ai-memory

- Evidencia: `~/workspace/ai-memory/LICENSE` contem MIT, copyright 2026 Fabio Akita.
- Compatibilidade com a premissa: parcial; a licenca raiz e permissiva, mas o escopo exato de uma eventual reutilizacao ainda nao foi auditado.
- Obrigacao para copia substancial: incluir o aviso de copyright e a permissao MIT.
- Dependencias e ativos: `deny.toml:16-42` configura `cargo-deny`, mas a politica nao foi executada nesta etapa; `crates/ai-memory-web/build.rs:20-69` e `crates/ai-memory-web/src/routes/statics.rs:9-14` mostram CSS/logo embutidos, e `companions/ai-memory-importer/Cargo.toml:1-25` nao declara licenca propria.
- Disposicao de licenca: todas as formas permanecem abertas; arquitetura e custo ainda precisam ser avaliados.

## Palinode

- Evidencia: `~/workspace/palinode/LICENSE` contem MIT, copyright 2026 Paul Kyle.
- Compatibilidade com a premissa: parcial; a licenca raiz e permissiva, mas o escopo exato de uma eventual reutilizacao ainda nao foi auditado.
- Obrigacao para copia substancial: incluir o aviso de copyright e a permissao MIT.
- Observacao: `claude-plugin/LICENSE` e `claude-plugin/.claude-plugin/plugin.json` confirmam MIT nesse subdiretorio. Dependencias Python/TypeScript, vetores identificados como CC BY 4.0 em `tests/test_amr_conformance.py:1-6` e avisos de codigo adaptado ainda devem ser inventariados antes de copia.
- Escopo verificado na Etapa 7: a fatia offline de parser, executor, hashing, quotes, claims e typed links foi extraida de arquivos cobertos pela MIT raiz. O runtime minimo usou `python-frontmatter==1.1.0` e `PyYAML==6.0.2`, ambos inventariados como MIT; seus textos e metadados estao na evidencia do spike.
- Exclusoes da fatia: plugin, prompts, vetores CC BY 4.0, modelos, ativos, API, store, indice, sessoes e integracoes nao foram aprovados para copia.
- Disposicao de licenca: fatia offline candidata a componente, em principio compativel mediante preservacao dos tres avisos MIT; a Etapa 8 ainda deve registrar a lista exata e o ADR deve aprovar qualquer incorporacao.

## claude-mem

- Evidencia: `~/workspace/claude-mem/LICENSE` contem Apache-2.0 e `~/workspace/claude-mem/NOTICE` identifica Claude-Mem, copyright 2026 Alex Newman.
- Compatibilidade com a premissa: parcial; a licenca raiz e permissiva, mas o escopo exato de uma eventual reutilizacao ainda nao foi auditado.
- Obrigacoes para redistribuicao: fornecer copia da Apache-2.0, marcar arquivos modificados, preservar avisos aplicaveis e reproduzir as atribuicoes aplicaveis do `NOTICE`.
- Excecao de escopo: `.claude-plugin/marketplace.json:9-21` inclui `claude-mem-cowork`, cujo `cowork/.claude-plugin/plugin.json:1-17` declara PolyForm-Noncommercial-1.0.0. Esse componente nao esta aprovado para incorporacao ao produto Apache-2.0.
- Ativos e dependencias: o clone nao possui inventario agregado para logos, fontes, slides, GIFs e bundle; `docs/license.md:26-29` tambem registra que Apache-2.0 nao concede direitos de marca.
- Disposicao de licenca: todas as formas permanecem abertas; privacidade, worker e persistencia de sessoes ainda precisam ser avaliados.

## claude-bedrock

- Evidencia: `~/workspace/claude-bedrock/LICENSE` contem MIT, copyright 2026 Iury Krieger.
- Compatibilidade com a premissa: parcial; a licenca raiz e permissiva, mas o escopo exato de uma eventual reutilizacao ainda nao foi auditado.
- Obrigacao para copia substancial: incluir o aviso de copyright e a permissao MIT.
- Dependencias e ativos: `skills/setup/SKILL.md:116-191` instala graphify e docling sem pin ou inventario de licencas. Nao foi encontrada proveniencia individual para `docs/banner.png` ou `site/public/videos/`; esses ativos ficam fora de reutilizacao ate auditoria.
- Disposicao de licenca: todas as formas permanecem abertas; a avaliacao determinara se ha um componente isolado ou apenas referencia.

## agent-memory

- Evidencia: `~/workspace/agent-memory/LICENSE` contem Apache-2.0, copyright 2026 Teterichev Anton.
- Compatibilidade com a premissa: parcial; a licenca raiz e permissiva, mas o escopo exato de uma eventual reutilizacao ainda nao foi auditado.
- Obrigacoes para redistribuicao: fornecer copia da Apache-2.0, marcar arquivos modificados e preservar avisos aplicaveis. Nenhum `NOTICE` raiz foi encontrado na revisao registrada.
- Dependencias e empacotamento: `go.mod:5-12` lista as dependencias diretas, mas o clone nao contem SBOM nem agregacao de notices. `npm/package.json:6-11` publica somente `bin/`, sem demonstrar que a licenca acompanha o tarball.
- Disposicao de licenca: todas as formas permanecem abertas; a separacao entre repositorio de codigo e memoria e o principal risco tecnico.

## twiceshy

- Evidencia: `~/workspace/twiceshy/LICENSE` contem o texto AGPL-3.0 e `README.md` identifica a escolha como `AGPL-3.0-only`.
- Compatibilidade com a premissa: falha para fork ou componente incorporado a distribuicao Apache-2.0.
- Uso permitido no plano atual: executar localmente para avaliacao, citar comportamentos e reimplementar ideias sem copiar expressao protegida.
- Escopo adicional: `README.md:20-24` informa que o corpus canonico e mantido em repositorio separado; ele nao estava no clone, portanto licenca e ativos nao foram auditados nem sao candidatos a reutilizacao nesta fase.
- Disposicao de licenca: referencia para quarentena, validacao, promocao e governanca.

## Gate para reutilizacao

Antes de incorporar qualquer componente, registrar no ADR e no inventario de terceiros:

1. arquivo ou pacote exato e revisao de origem;
2. licenca aplicavel ao caminho, incluindo excecoes ou licencas de subdiretorios;
3. avisos e `NOTICE` que devem acompanhar a distribuicao;
4. dependencias transitivas e ativos nao cobertos pela licenca do codigo;
5. modificacoes realizadas e forma de atribuicao;
6. confirmacao de que a distribuicao final continua sob Apache-2.0.
