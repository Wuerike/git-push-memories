# Features

O GPM mantém conhecimento compartilhado em arquivos Markdown e usa GitHub para
captura e publicação. As skills interpretam o conteúdo e conduzem a revisão.
O percurso de uso está em [workflow.md](workflow.md), e a instalação da
integração em [Claude Code](../integrations/claude-code/README.md).

## Repository model

A `main` contém a memória publicada em `memories/`. A branch `proposals` contém
arquivos Markdown pendentes sob `proposals/`, inclusive arquivos criados
manualmente. Cada consolidação usa uma branch `promote/<UTC>-<random>` baseada
na `main` obtida no fetch. Os insumos selecionados ficam em uma revisão fixa,
copiados para fora do worktree editável.

A memória é Markdown comum. O agente decide headings, metadados, nomes de páginas,
links e organização interna. As categorias disponíveis são as de `gpm.json`:

```json
{ "schema_version": 1, "memory_categories": ["domains", "decision"] }
```

O agente escolhe destinos apenas nessas categorias. Quando nenhuma serve ao
assunto, discute uma alteração de configuração com a pessoa. A leitura usa
somente `memories/<categoria-configurada>/**/*.md` da `main` publicada.

## Installation and CLI

`install.sh` e `make install` instalam `gpm`, por padrão em `~/.local/bin`.

| Comando | Efeito |
| --- | --- |
| `gpm setup <url> [--categories a,b] [--yes]` | Prepara clone/worktrees, configura `gpm.json`, registra MCP e instala skills no escopo do usuário |
| `gpm promote --finish` | Confirma merge pelo GitHub, limpa propostas selecionadas e atualiza a leitura local |
| `gpm mcp` | Serve as oito ferramentas MCP por stdio |
| `gpm version`, `gpm help` | Informam versão e ajuda |

O setup exige `git`, `claude`, identidade Git e credenciais para o remoto.
Categorias são exigidas para inicializar um remoto vazio. Um remoto com conteúdo
precisa de `gpm.json` na `main`. As categorias devem ser uma lista não vazia,
sem duplicatas e com nomes lowercase kebab-case. O arquivo usa `schema_version: 1`
e os campos mostrados acima.

A URL é salva em `<UserConfigDir>/git-push-memories/config.json`, no campo
`repo_url`; o setup usa a mesma URL em execuções subsequentes. As skills são
instaladas em `~/.claude/skills`. O clone bare e os worktrees ficam no diretório
de estado do usuário.

## Skills

`/remember` investiga o assunto nas memórias publicadas e propostas pendentes,
interpreta o conhecimento e sugere ação e destino. O agente adapta buscas e
leituras à dúvida, mostra o texto exato que será compartilhado e pede confirmação
antes de criar a proposta pela API do GitHub.

`/promote [assunto ou caminhos]` seleciona propostas, investiga as memórias
relacionadas e discute um plano global na conversa. O agente lê integralmente
as propostas selecionadas e as páginas que pretende editar; segue referências e
contradições conforme necessário. Explica incertezas e limitações de cobertura.
A profundidade da investigação acompanha o escopo da consolidação.

A pessoa aprova o plano antes da edição e os arquivos/diff atuais antes da
publicação. O agente escreve diretamente no worktree, pode delegar arquivos
independentes e revisa a coerência do conjunto. Durante a edição humana, os
agentes suspendem a escrita. O envio cria um commit com os caminhos escolhidos
e abre PR para `main`. O merge é humano.

Cada proposta selecionada recebe destinos ou justificativa de exclusão no resumo.
Uma proposta pode alimentar várias memórias, e várias podem alimentar uma página.
Propostas fora da seleção continuam pendentes.

## MCP tools

| Ferramenta | Entrada | Resultado/efeito |
| --- | --- | --- |
| `memory_search` | `query`, `path_prefix` e `limit` opcionais | Trechos de texto com caminho, intervalo de linhas e revisão Git |
| `memory_read` | `path`; `revision`, `start_line` e `line_count` opcionais | Conteúdo do arquivo ou janela de linhas na revisão publicada |
| `memory_proposal_list` | `{}` | Categorias e propostas com `path`, `blob` e `content` |
| `memory_propose` | `title`, `content` | Cria arquivo e commit na `proposals` remota pela API do GitHub |
| `memory_promote_prepare` | `proposal_paths` opcional | Fetch, worktree, categorias, revisões e inventários dos insumos selecionados |
| `memory_promote_check` | `{}` | Caminhos alterados e diff do Git, somente leitura |
| `memory_promote_submit` | `summary`, `paths` opcional | Commit das memórias escolhidas, envio da branch e criação de PR |
| `memory_promote_finish` | `{}` | Confirma merge, limpa propostas selecionadas que ainda correspondem aos blobs e atualiza leitura |

A busca textual encontra qualquer termo da consulta, sem distinguir maiúsculas,
e também considera o caminho. Retorna um trecho de até 20 linhas por arquivo,
a partir de até três linhas antes da primeira correspondência. A ordem é por
caminho. `limit` vale 20 por padrão, com máximo de 100 arquivos; `truncated`
indica resultados adicionais. Os trechos orientam a investigação do agente.

`memory_read` usa linhas a partir de 1. `start_line` vale 1 e `line_count` vale
200 por padrão, com máximo de 2000 linhas por chamada. Continue a leitura enquanto
`truncated` for verdadeiro. Use `revision` retornada pela busca para ler aquele
mesmo estado, mesmo após atualizações. Sem revisão explícita, lê o `HEAD` do
checkout publicado. Revisões explícitas precisam pertencer ao histórico desse
checkout. Arquivos são lidos de objetos Git, preservando seu conteúdo.

A listagem de propostas fornece identidade por caminho e blob, com conteúdo
integral. Títulos, assuntos, ações e agrupamentos são interpretados pelo agente.
A captura gera um nome único `proposals/<id>-<slug>.md` e retorna também ID,
título, caminho e commit da criação.

A preparação retorna `worktree`, `inputs_root`, `branch`, `main_sha`,
`proposals_sha`, `categories`, `memory_paths` e `proposals`. Cada proposta tem
`path`, `blob` e `input_path`. Caminhos de insumos são absolutos; caminhos de
memória são relativos ao worktree. Omitir `proposal_paths` seleciona todas as
propostas disponíveis. Uma publicação anterior mergeada é concluída primeiro;
um PR anterior aberto retorna `merge_pending`.

A conferência inclui adições, edições e exclusões de Markdown sob `memories/`.
Usa um índice Git temporário fora do worktree e conserva os arquivos e o índice
de trabalho. A revisão editorial, inclusive a escolha de categorias, cabe ao
agente e à pessoa. O envio recebe os caminhos revisados em `paths`; se omitidos,
envia todas as memórias alteradas. Os blobs de memória preservam os bytes dos
arquivos, inclusive finais de linha, usando os caminhos revisados. Retorna
`commit`, `pr_number` e `pr_url`.

O registro local `promotion-published.json` liga PR, branch e caminhos/blobs
selecionados à limpeza. A conclusão retorna `found`, `merged`, `pr_url`, `removed`
e `preserved`. Propostas novas, alteradas ou fora da seleção ficam pendentes.

As respostas usam `schema_version: 1`, com `data` ou `error`. Erros incluem uma
mensagem e códigos como `promotion_not_prepared`, `no_proposals`, `no_changes`,
`invalid_summary`, `invalid_path`, `invalid_scope`, `invalid_query`,
`invalid_range`, `invalid_proposal`, `merge_pending`, `gh_unavailable`,
`push_denied`, `network_unavailable`, `worktree_dirty` e `operation_failed`.

## Operational requirements

- Captura e publicação exigem GitHub, rede e `gh` autenticado com escrita.
- A leitura atualiza a `main` com TTL de 10 minutos e conserva o último estado
  conhecido quando o fetch falha. A conclusão após merge atualiza a leitura.
- Execute uma promoção por vez. O estado de preparação pertence ao processo MCP;
  conserve o caminho do worktree para consultar os arquivos após uma interrupção.
- Um diff de memórias vazio deixa as propostas pendentes.
- A confirmação humana decide o compartilhamento de texto livre. O agente deve
  excluir segredos, dados pessoais privados e material de sessão. Propostas
  enviadas permanecem no histórico Git depois da exclusão dos arquivos.
- Falhas remotas podem exigir inspeção do estado antes de repetir uma operação.
  Os arquivos preparados permanecem disponíveis para revisão.
- O suporte distribuído é Linux `amd64` e `arm64`.

## Verification

```sh
make verify
make verify-offline
```

Os comandos executam testes, race detector, vet e build. A verificação de race
exige compilador C para CGO. `go test ./tests/e2e` executa o binário real com Git
local e GitHub simulado. O script `tests/e2e/run-phase-3.sh` registra evidências
em um worktree limpo. Testes das instruções das skills verificam o contrato
escrito; o julgamento do agente exige revisão de cenários.
