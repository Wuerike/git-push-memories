# Licencas dos candidatos da Fase 1

Registro feito a partir dos arquivos presentes nas revisoes locais fixadas em 2026-09-04. Este documento orienta a avaliacao tecnica e nao substitui revisao juridica quando houver incorporacao de codigo ou distribuicao.

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
- Disposicao de licenca: referencia; eventual reutilizacao do plugin depende do spike de esclarecimento ja previsto no plano.

## ai-memory

- Evidencia: `~/workspace/ai-memory/LICENSE` contem MIT, copyright 2026 Fabio Akita.
- Compatibilidade com a premissa: parcial; a licenca raiz e permissiva, mas o escopo exato de uma eventual reutilizacao ainda nao foi auditado.
- Obrigacao para copia substancial: incluir o aviso de copyright e a permissao MIT.
- Disposicao de licenca: todas as formas permanecem abertas; arquitetura e custo ainda precisam ser avaliados.

## Palinode

- Evidencia: `~/workspace/palinode/LICENSE` contem MIT, copyright 2026 Paul Kyle.
- Compatibilidade com a premissa: parcial; a licenca raiz e permissiva, mas o escopo exato de uma eventual reutilizacao ainda nao foi auditado.
- Obrigacao para copia substancial: incluir o aviso de copyright e a permissao MIT.
- Observacao: o subdiretorio `claude-plugin` tambem possui um `LICENSE`; qualquer componente escolhido deve ter seu escopo confirmado no relatorio.
- Disposicao de licenca: todas as formas permanecem abertas; isolamento do componente ainda precisa ser demonstrado.

## claude-mem

- Evidencia: `~/workspace/claude-mem/LICENSE` contem Apache-2.0 e `~/workspace/claude-mem/NOTICE` identifica Claude-Mem, copyright 2026 Alex Newman.
- Compatibilidade com a premissa: parcial; a licenca raiz e permissiva, mas o escopo exato de uma eventual reutilizacao ainda nao foi auditado.
- Obrigacoes para redistribuicao: fornecer copia da Apache-2.0, marcar arquivos modificados, preservar avisos aplicaveis e reproduzir as atribuicoes aplicaveis do `NOTICE`.
- Disposicao de licenca: todas as formas permanecem abertas; privacidade, worker e persistencia de sessoes ainda precisam ser avaliados.

## claude-bedrock

- Evidencia: `~/workspace/claude-bedrock/LICENSE` contem MIT, copyright 2026 Iury Krieger.
- Compatibilidade com a premissa: parcial; a licenca raiz e permissiva, mas o escopo exato de uma eventual reutilizacao ainda nao foi auditado.
- Obrigacao para copia substancial: incluir o aviso de copyright e a permissao MIT.
- Disposicao de licenca: todas as formas permanecem abertas; a avaliacao determinara se ha um componente isolado ou apenas referencia.

## agent-memory

- Evidencia: `~/workspace/agent-memory/LICENSE` contem Apache-2.0, copyright 2026 Teterichev Anton.
- Compatibilidade com a premissa: parcial; a licenca raiz e permissiva, mas o escopo exato de uma eventual reutilizacao ainda nao foi auditado.
- Obrigacoes para redistribuicao: fornecer copia da Apache-2.0, marcar arquivos modificados e preservar avisos aplicaveis. Nenhum `NOTICE` raiz foi encontrado na revisao registrada.
- Disposicao de licenca: todas as formas permanecem abertas; a separacao entre repositorio de codigo e memoria e o principal risco tecnico.

## twiceshy

- Evidencia: `~/workspace/twiceshy/LICENSE` contem o texto AGPL-3.0 e `README.md` identifica a escolha como `AGPL-3.0-only`.
- Compatibilidade com a premissa: falha para fork ou componente incorporado a distribuicao Apache-2.0.
- Uso permitido no plano atual: executar localmente para avaliacao, citar comportamentos e reimplementar ideias sem copiar expressao protegida.
- Disposicao de licenca: referencia para quarentena, validacao, promocao e governanca.

## Gate para reutilizacao

Antes de incorporar qualquer componente, registrar no ADR e no inventario de terceiros:

1. arquivo ou pacote exato e revisao de origem;
2. licenca aplicavel ao caminho, incluindo excecoes ou licencas de subdiretorios;
3. avisos e `NOTICE` que devem acompanhar a distribuicao;
4. dependencias transitivas e ativos nao cobertos pela licenca do codigo;
5. modificacoes realizadas e forma de atribuicao;
6. confirmacao de que a distribuicao final continua sob Apache-2.0.
