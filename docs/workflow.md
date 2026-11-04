# GPM workflow

O GPM move conhecimento da conversa para propostas, consolida arquivos Markdown
com revisão humana e publica memórias por PR. A leitura cotidiana usa a `main`.

## Branches and files

| Branch | Conteúdo | Escrita |
| --- | --- | --- |
| `main` | `gpm.json` e memória publicada em `memories/` | Merge humano de PR |
| `proposals` | Arquivos Markdown pendentes sob `proposals/` | Captura, edição manual e limpeza após merge |
| `promote/<UTC>-<random>` | Arquivos finais da consolidação sobre a `main` | Agente e pessoa no mesmo worktree |

As categorias em `gpm.json` orientam os destinos disponíveis ao agente. Uma página
pode usar o Markdown, os metadados e a estrutura adequados ao assunto.

## Setup

Prepare um repositório vazio no GitHub e execute:

```sh
gpm setup https://github.com/your-team/memories.git --categories technical,product
```

O setup exige `git` com worktrees, CLI do `claude`, identidade Git e credenciais
para o remoto. Instala o MCP e as skills `/remember` e `/promote` no escopo do
usuário. Captura e publicação usam `gh` autenticado com escrita. Quem apenas lê
um repositório preparado usa as credenciais Git para atualizá-lo.

Em remoto vazio, o setup cria `gpm.json`, `main` e `proposals`. Em um repositório
preparado, usa as categorias existentes na `main`. `--yes` permite executar sem
prompts. Configure a proteção da `main` para exigir PR depois da inicialização,
que precisa enviar o primeiro commit.

## Local workspace

O clone bare e os worktrees ficam em
`$XDG_STATE_HOME/git-push-memories/<repo-id>` ou, por padrão,
`~/.local/state/git-push-memories/<repo-id>`.

| Caminho no estado | Uso |
| --- | --- |
| `repo` | Clone bare e refs remotas |
| `read` | Checkout publicado de `origin/main` |
| `proposals` | Consulta local e limpeza de propostas |
| `promotions/<id>/worktree` | Arquivos finais editáveis |
| `promotions/<id>/inputs` | Cópias dos insumos selecionados |
| `promotion-published.json` | PR, branch e caminhos/blobs selecionados para limpeza |

O projeto de código em que você trabalha mantém seu próprio repositório.
O estado de edição da consolidação pertence ao processo MCP. Os arquivos ficam
no disco para consulta após uma interrupção.

## Published reads

`memory_search` pesquisa texto nas categorias configuradas e retorna caminhos,
linhas e uma revisão Git. Use `memory_read` com essa revisão para investigar
os arquivos relacionados no mesmo estado. A leitura aceita janelas de linhas;
continue enquanto `truncated` indicar conteúdo restante.

A busca serve como ponto de partida. O agente escolhe consultas, segue referências
e aprofunda a leitura conforme as dúvidas da tarefa. Propostas e trabalho em
revisão ficam fora da leitura publicada.

O checkout `read` faz fetch assíncrono com TTL de 10 minutos. Uma tentativa,
inclusive falha, atualiza o horário do TTL. Sem rede, a leitura conserva o último
estado conhecido. Objetos Git permitem continuar lendo uma revisão anterior
publicada após a atualização.

## Capture with /remember

Invoque `/remember <fato>`. O agente interpreta o assunto e consulta a memória
e as propostas pendentes. Usa apenas as categorias de `gpm.json` ao sugerir um
destino e explica inferências ou ambiguidades.

Revise os campos `title` e `content` e confirme o envio ao repositório compartilhado.
O conteúdo é Markdown livre com conhecimento, contexto e sugestão de ação ou
destino quando conhecidos. A captura cria arquivo e commit na branch remota
`proposals` pela API do GitHub e retorna caminho e commit.

Uma falha é informada com o estado conhecido. Quando o resultado remoto for
incerto, confira a listagem antes de repetir a captura.

## Consolidate with /promote

Execute uma promoção por vez, com rede, identidade Git e `gh` autenticado.

1. **Selecione o escopo.** Invoque `/promote` para considerar as propostas
   pendentes ou indique um assunto ou caminhos. O agente consulta
   `memory_proposal_list` e seleciona os insumos relacionados. Arquivos manuais
   Markdown sob `proposals/` também participam. Caminho e blob identificam cada
   proposta; o agente interpreta seu título e assunto.
2. **Prepare os arquivos.** `memory_promote_prepare` aceita `proposal_paths`
   para um subconjunto e seleciona todas quando o argumento é omitido. Faz fetch,
   cria o worktree sobre a `main` e copia os insumos para fora do worktree.
   O conjunto inicial fica fixo. A preparação conclui uma publicação anterior
   mergeada; um PR anterior aberto informa `merge_pending`.
3. **Investigue e aprove o plano.** O agente lê integralmente as propostas
   selecionadas e as páginas que pretende alterar. Explora conhecimento
   relacionado, referências e contradições conforme o escopo e explica limites
   da análise. Discuta o plano global na conversa: grupos, destinos e mudanças.
   Cada proposta selecionada recebe destinos ou justificativa de exclusão.
   Aprove o plano antes de qualquer edição de memória.
4. **Prepare o conteúdo final.** O agente escreve páginas Markdown completas
   apenas nas categorias configuradas, preservando conhecimento válido e
   restringindo reorganizações ao plano. Pode delegar arquivos independentes,
   com um responsável por arquivo e contexto suficiente. O principal espera
   todos e revisa a coerência entre páginas. Mudanças materiais voltam à discussão.
5. **Confira e revise.** `memory_promote_check` retorna os caminhos alterados e
   o diff atual produzido pelo Git, incluindo arquivos novos e exclusões. A
   conferência conserva arquivos e índice do worktree. O agente mostra resumo,
   diff por memória e caminho para abrir no VS Code, se desejar. Durante sua
   edição, os agentes suspendem a escrita. Ajustes solicitados partem dos
   arquivos atuais e preservam suas edições fora do pedido.
6. **Aprove a publicação.** Depois da revisão, o agente relê os arquivos e confere
   o diff atual. Mudanças exigem nova revisão. Com sua aprovação,
   `memory_promote_submit` recebe `summary` e os `paths` revisados, cria um commit
   com essas memórias, envia a branch e abre PR para `main`. O resumo registra
   destinos ou justificativas de todas as propostas selecionadas.
7. **Faça o merge humano.** Revise o PR e faça merge no GitHub. As propostas
   selecionadas ficam pendentes até a confirmação do merge.
8. **Conclua a limpeza.** Na máquina que publicou, execute
   `gpm promote --finish`, peça `memory_promote_finish` ou invoque o próximo
   `/promote`. A conclusão confirma o merge e o commit em `origin/main`, remove
   somente caminhos selecionados cujo blob continua igual e atualiza a leitura
   publicada. Propostas novas, alteradas ou fora da seleção permanecem pendentes.

Um diff de memórias vazio deixa as propostas pendentes. A limpeza usa o registro
local de publicação. Falhas são informadas e conservam os arquivos preparados.
