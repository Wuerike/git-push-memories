# GPM workflow

O GPM move conhecimento da conversa para propostas, consolida arquivos finais
com revisão humana e publica memórias por PR. A leitura cotidiana usa a `main`.

## Branches and files

| Branch | Conteúdo | Escrita |
| --- | --- | --- |
| `main` | `gpm.json` e memórias canônicas em `memories/**` | Merge humano de PR |
| `proposals` | Propostas pendentes em `proposals/<id>-<slug>.md` | Captura e limpeza após merge |
| `promote/<UTC>-<random>` | Base da `main` com os arquivos finais da consolidação | Agente e pessoa no mesmo worktree |

Cada consolidação começa na revisão obtida de `origin/main`. Os insumos vêm de
uma revisão fixa de `origin/proposals`, copiados integralmente para fora do
worktree. Propostas e arquivos auxiliares ficam fora do commit canônico.
Propostas recebidas depois da preparação ficam para a próxima consolidação.

## Setup

Prepare um repositório vazio no GitHub e execute:

```sh
gpm setup https://github.com/your-team/memories.git --categories technical,product
```

O setup exige `git` com worktrees, CLI do `claude`, identidade Git e credenciais
para o remoto. Instala o MCP e as skills `/remember` e `/promote` no escopo do
usuário. O `gh` autenticado com escrita é necessário para capturar e publicar;
quem só lê um repositório preparado não precisa dele.

Em remoto vazio, o setup cria `gpm.json`, `main` e `proposals`. Se a `main` já
contém um `gpm.json` válido, usa as categorias existentes e cria `proposals`
apenas se faltar. Um remoto com conteúdo sem `gpm.json` na `main` é rejeitado.
Categorias são exigidas na inicialização; `--yes` permite executar sem prompts.
A URL precisa corresponder exatamente à configuração existente.

Configure a proteção da `main` para exigir PR depois do setup: a inicialização
precisa enviar o primeiro commit. O GPM não configura regras do GitHub.

## Local workspace

O clone bare e os worktrees ficam em
`$XDG_STATE_HOME/git-push-memories/<repo-id>` ou, por padrão,
`~/.local/state/git-push-memories/<repo-id>`. O repositório do projeto em que a
pessoa trabalha não é usado como repositório de memória.

| Caminho no estado | Uso |
| --- | --- |
| `repo` | Clone bare e refs remotas |
| `read` | Leitura publicada em `origin/main` |
| `proposals` | Listagem local e limpeza das propostas |
| `promotions/<id>/worktree` | Arquivos finais editáveis |
| `promotions/<id>/inputs` | Cópias dos insumos selecionados |
| `promotion-published.json` | Vínculo local entre PR, branch e caminhos/blobs das propostas |

O registro de publicação permite concluir a limpeza após merge. Não guarda
plano, conteúdo de trabalho ou sessão de edição. Worktrees preparados permanecem
no disco; esta versão não retoma uma consolidação interrompida.

## Published reads

`memory_search` e `memory_expand` leem a `main` publicada. Propostas pendentes e
arquivos em revisão não entram na busca.

O worktree `read` faz fetch com TTL de 10 minutos. A leitura usa o estado
atual e dispara atualização assíncrona se necessário. Uma tentativa de fetch,
inclusive falha, atualiza o horário do TTL.
Sem rede, a leitura conserva a última versão conhecida. Handles de conteúdo
alterado são rejeitados como obsoletos na expansão.

## Capture with /remember

Invoque `/remember` e confirme o texto que será enviado ao repositório
compartilhado. A proposta tem título e conteúdo Markdown livres. O código não
interpreta instruções de ação ou destino.

A captura cria arquivo e commit na `proposals` remota pela API do GitHub, sem
worktree de escrita local. O retry existente para conflito de branch fica
restrito à captura. Uma falha é informada à pessoa; captura exige rede e acesso.
`memory_proposal_list` atualiza a listagem local em modo best-effort e pode
mostrar o último estado conhecido quando o fetch falha.

## Consolidate with /promote

Execute uma promoção por vez, com ambiente preparado, rede, identidade Git e
`gh` autenticado. O agente conduz as decisões na conversa e usa ferramentas
pequenas para preparar, conferir, publicar e concluir.

1. **Prepare os insumos.** `memory_promote_prepare` faz fetch, cria o worktree
   sobre a `main` e retorna inventários completos e os caminhos das propostas.
   Se houver publicação anterior já mergeada, conclui sua limpeza primeiro.
   Um PR anterior ainda aberto impede preparar outra publicação.
2. **Leia o conjunto integral.** O agente lê todas as propostas e todas as
   memórias da base. Busca serve apenas para navegação. Se o contexto não
   comportar o conjunto completo, informa a limitação e para.
3. **Aprove o plano global na conversa.** O plano relaciona grupos, propostas,
   destinos e mudanças. Cada proposta recebe destinos ou justificativa livre
   de descarte. Uma proposta pode alimentar várias memórias, e várias podem
   alimentar uma só. Nenhum arquivo canônico é editado antes dessa aprovação.
4. **Prepare os arquivos finais.** O agente escreve conhecimento canônico no
   worktree, preservando fatos válidos e restringindo reorganização ao plano.
   Pode delegar arquivos independentes, com um responsável por arquivo e
   contexto completo; o principal espera todos e revisa coerência, duplicidades,
   contradições, referências e omissões. Mudanças materiais voltam à conversa.
5. **Confira e revise.** `memory_promote_check` preserva IDs existentes, completa
   IDs de headings novos, valida categorias/formato e devolve o diff atual.
   O agente apresenta resumo global, diff por memória e caminho do worktree.
   Você pode editar os mesmos arquivos no VS Code; o editor não abre sozinho.
   Durante a edição humana, todos os agentes suspendem a escrita e a conferência.
   Ajustes posteriores partem dos arquivos atuais e preservam suas edições.
6. **Aprove a publicação do diff atual.** Depois da revisão, o agente relê os
   arquivos. Qualquer mudança exige nova revisão e aprovação. Com a aprovação,
   `memory_promote_submit` recebe o resumo global, cria um commit global apenas
   com alterações canônicas, envia a branch e abre um PR para `main`. O corpo do
   PR registra destinos ou justificativas das propostas. O agente devolve o link.
7. **Faça o merge humano.** Revise o PR e faça merge no GitHub. Até a confirmação
   do merge, todas as propostas concluídas permanecem na `proposals`.
8. **Conclua a limpeza.** Na mesma máquina e com o registro de publicação,
   execute `gpm promote --finish`, peça `memory_promote_finish` ou invoque o
   próximo `/promote`. A conclusão confirma o merge no GitHub e a presença do
   commit publicado em `origin/main`. Remove somente propostas concluídas cujo
   caminho e blob ainda correspondam ao conjunto inicial. Propostas novas ou
   alteradas são preservadas. Depois atualiza a leitura local para a `main`.

## Current limits

Uma falha interrompe o fluxo e informa o erro. Os arquivos preparados permanecem
no disco para consulta. Com um diff canônico vazio, as propostas permanecem
pendentes para uma próxima consolidação.
