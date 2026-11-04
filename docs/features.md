# Features

Esta referência descreve a implementação atual. O percurso de uso está em
[workflow.md](workflow.md), e a integração em
[Claude Code](../integrations/claude-code/README.md).

## Repository model

O GPM usa um repositório de memória por pessoa. A `main` contém as memórias
publicadas; `proposals` contém propostas pendentes. Cada consolidação cria uma
branch `promote/<UTC>-<random>` a partir da `main` obtida no fetch. Insumos de uma
revisão fixa de propostas ficam fora do worktree editável.

Só `memories/<categoria-configurada>/**/*.md` participa da leitura. O clone bare
fica no estado do usuário, com worktrees separados. O MCP usa stdio.

## Installation and CLI

`install.sh` e `make install` instalam `gpm`, por padrão em `~/.local/bin`.

| Comando | Efeito |
| --- | --- |
| `gpm setup <url> [--categories a,b] [--yes]` | Prepara clone/worktrees, valida ou cria `gpm.json`, configura MCP e instala skills no escopo do usuário |
| `gpm promote --finish` | Confirma merge pelo GitHub, limpa propostas correspondentes e atualiza a leitura local |
| `gpm mcp` | Serve as oito ferramentas MCP por stdio |
| `gpm version`, `gpm help` | Informam versão e ajuda sem configuração |

O setup exige `git`, `claude`, identidade Git e credenciais para o remoto. Em
remoto vazio, exige categorias para criar `gpm.json`, `main` e `proposals`.
Um remoto com conteúdo precisa de `gpm.json` válido na `main`. A URL configurada
é salva em `<UserConfigDir>/git-push-memories/config.json`, no campo `repo_url`;
URLs diferentes são rejeitadas. A instalação das skills usa `~/.claude/skills`.

O arquivo de categorias tem este formato:

```json
{ "schema_version": 1, "memory_categories": ["domains", "decision"] }
```

A lista precisa ser não vazia, sem duplicatas, com nomes lowercase kebab-case.
Campos desconhecidos e conteúdo extra são rejeitados.

A conclusão exige o registro local `promotion-published.json` criado no envio.
Sem publicação pendente ou com PR ainda aberto, a CLI informa a situação e sai
com erro.

## Explicit Claude Code commands

`/remember` pesquisa memórias e propostas, sugere ação e destino no texto livre,
mostra o que será compartilhado e espera confirmação antes de criar a proposta
pela API do GitHub. Captura não altera memória canônica.

`/promote` lê todas as propostas e memórias da base, discute o plano global na
conversa e espera aprovação antes de editar. O agente escreve arquivos finais,
pode delegar memórias independentes e revisa a coerência do conjunto. A pessoa
revê o diff atual e pode editar os mesmos arquivos. Os agentes param de escrever
durante a revisão humana. Uma segunda aprovação autoriza um commit global
canônico e PR para `main`; o merge é humano.

Cada proposta tem destinos ou justificativa livre no resumo do PR. São aceitas
relações de várias propostas para uma memória e de uma proposta para várias
memórias. O agente e a pessoa decidem o significado da consolidação.

## MCP tools

Há quatro ferramentas de captura/leitura e quatro de promoção:

| Ferramenta | Entrada | Resultado/efeito |
| --- | --- | --- |
| `memory_search` | `query`, `budget_bytes`, `path_prefix` opcional, `source` opcional | Busca textual com trechos e handles expansíveis |
| `memory_expand` | `handle`, `budget_bytes`, `source` opcional | Expande uma seção publicada dentro do orçamento |
| `memory_proposal_list` | `source` opcional | Categorias e propostas com ID, caminho, título e conteúdo livres |
| `memory_propose` | `title`, `content` | Cria arquivo e commit na `proposals` remota pela API do GitHub |
| `memory_promote_prepare` | `{}` | Fetch, worktree sobre a `main`, categorias, revisões e inventários completos |
| `memory_promote_check` | `{}` | Completa IDs, valida arquivos atuais e retorna caminhos alterados e diff; não cria commit |
| `memory_promote_submit` | `summary` | Cria um commit global só com memórias, envia branch e abre PR com o resumo aprovado |
| `memory_promote_finish` | `{}` | Confirma merge, remove propostas de caminho/blob correspondentes e atualiza leitura |

As ferramentas de leitura aceitam somente `source: main` (padrão). A listagem de
propostas aceita somente `source: proposals` (padrão). Outros seletores são
rejeitados. Os arquivos da consolidação são lidos e editados por ferramentas
nativas no caminho retornado, sujeito às permissões normais da sessão.

A preparação retorna `worktree`, `inputs_root`, `branch`, `main_sha`,
`proposals_sha`, `categories`, `memory_paths` e `proposals`. Cada proposta tem
`id`, `path`, `title`, `blob` e `input_path`. Caminhos de insumos são absolutos;
caminhos de memória são relativos ao worktree. O conjunto inicial fica fixo.
A preparação conclui uma publicação anterior mergeada antes de criar outra;
se o PR ainda estiver aberto, informa `merge_pending`.

O envio retorna `commit`, `pr_number` e `pr_url`. O registro local conserva
somente o vínculo entre PR, branch e caminhos/blobs de propostas para conclusão.
A conclusão retorna `found`, `merged`, `pr_url`, `removed` e `preserved`.
Propostas novas ou alteradas permanecem pendentes após a limpeza.

As respostas usam `schema_version: 1`, com `data` ou `error`. Falhas de promoção
incluem `promotion_not_prepared`, `no_proposals`, `no_changes`,
`invalid_summary`, `invalid_memory`, `merge_pending`, `gh_unavailable`,
`push_denied`, `network_unavailable`, `worktree_dirty` e `promotion_failed`.
Uma falha interrompe o percurso sem declarar sucesso ou apagar arquivos
preparados automaticamente.

## Constraints

- **Revisão:** aprovação de plano, preservação de edições humanas e aprovação do
  diff atual são conduzidas pela skill e pela pessoa. O código valida formato e
  Git; a qualidade semântica depende dessa revisão.
- **Formato:** somente Markdown canônico nas categorias configuradas é enviado.
  IDs existentes permanecem; headings novos recebem IDs na conferência.
  Arquivos auxiliares e cópias de propostas ficam fora do commit.
- **Leitura:** usa `main`, com TTL de 10 minutos e último estado conhecido quando
  o fetch falha. A conclusão após merge atualiza a leitura local imediatamente.
- **Privacidade:** o código não filtra segredos ou dados pessoais do texto livre.
  A confirmação humana decide o compartilhamento. Propostas enviadas continuam
  no histórico Git mesmo depois da remoção do arquivo. Origem relevante pode
  ser escrita no texto; não há avaliação estruturada de fontes.
- **Dependências:** captura e publicação exigem GitHub, `gh` autenticado, rede e
  escrita no repositório. Suporte distribuído: Linux `amd64` e `arm64`.
- **Escopo:** execute uma promoção por vez. A consolidação exige a leitura de
  todas as propostas e memórias da base no contexto disponível. Com um diff
  canônico vazio, as propostas permanecem pendentes.

## Verification

```sh
make verify              # testes, race, vet e build
make verify-offline      # mesmas verificações sem proxy de módulos
tests/e2e/run-phase-3.sh # binário real com Git local e GitHub simulado
```

A verificação de race exige um compilador C para CGO. Os testes de contrato da
skill verificam as instruções escritas; não executam julgamento por um LLM.
