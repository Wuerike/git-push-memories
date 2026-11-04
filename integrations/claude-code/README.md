# Claude Code

A integração oferece MCP por stdio e as skills explícitas `/remember` e `/promote`
para capturar e consolidar conhecimento compartilhado.

## Setup

Depois de instalar a CLI, execute por máquina:

```sh
gpm setup git@github.com:org/memoria.git --categories domains,decision
```

O setup prepara o clone bare e os worktrees `read` e `proposals` no diretório de
estado, salva a URL, registra `gpm mcp` no escopo `user` e instala
`~/.claude/skills/{remember,promote}/SKILL.md`. Repita o setup para reparar a
instalação ou atualizar o caminho do binário.

As categorias de `gpm.json` são as opções disponíveis ao agente para organizar
a memória. A leitura usa `memories/<categoria-configurada>/**/*.md` da `main`.
Veja os requisitos no [guia do fluxo](../../docs/workflow.md).

## Capture

Em uma sessão, invoque:

```text
/remember <fato>
```

O agente investiga a memória e as propostas pendentes, interpreta o assunto e
recomenda uma ação. Escolhe consultas e aprofunda a leitura conforme necessário,
explicando ambiguidades e limites da investigação. Ao sugerir um destino, usa
apenas as categorias configuradas.

Revise a interpretação e os campos exatos `title` e `content`. Sua confirmação
autoriza o envio ao repositório compartilhado. O conteúdo é Markdown livre.
A captura cria arquivo e commit na branch remota `proposals` pela API do GitHub,
com rede e `gh` autenticado. O agente retorna caminho e commit.

## Consolidate

Invoque:

```text
/promote
/promote regras de cobrança
/promote proposals/billing.md proposals/ownership.md
```

O agente seleciona propostas conforme o escopo solicitado e prepara um worktree
sobre a `main`. Cópias integrais dos insumos selecionados ficam em uma revisão
fixa, fora do worktree. As demais propostas permanecem pendentes.

Discuta o plano global na conversa: grupos, destinos e justificativas para cada
proposta selecionada. O agente lê integralmente os insumos e as páginas que
pretende editar. Investiga memórias relacionadas e referências conforme o assunto;
reorganizações amplas podem exigir leitura de todo o inventário. Explica
incertezas e limitações de cobertura.

Aprove o plano antes da escrita. O agente prepara páginas Markdown completas,
com headings, metadados e organização adequados ao conhecimento. Os destinos
usam as categorias de `gpm.json`. Pode delegar arquivos independentes e revisa
a coerência do conjunto. Mudanças materiais exigem nova decisão na conversa.

A conferência retorna caminhos alterados e o diff do Git, incluindo arquivos
novos e exclusões, conservando os arquivos e o índice de trabalho. O agente
mostra resumo, diff por memória e caminho do worktree. Você pode editar esses
mesmos arquivos no VS Code. Durante a edição humana, os agentes suspendem a
escrita. Diga quando terminar ou quais ajustes deseja; ajustes preservam suas
edições fora do pedido.

Aprove a publicação dos arquivos e do diff atuais. Mudanças após a revisão
exigem nova aprovação. O envio cria um commit com os caminhos revisados e abre
PR para `main`, com resumo dos destinos ou justificativas de todas as propostas
selecionadas. Revise e faça o merge no GitHub.

Depois do merge, na máquina que publicou, execute:

```sh
gpm promote --finish
```

Você também pode pedir `memory_promote_finish` ou invocar o próximo `/promote`.
A conclusão confirma o merge e o commit publicado, remove apenas os caminhos
selecionados cujos blobs ainda correspondem aos insumos e atualiza a leitura.
Propostas novas, alteradas ou fora da seleção permanecem pendentes. O registro
local `promotion-published.json` permite essa limpeza. Execute uma promoção por
vez. Um diff de memórias vazio deixa os insumos pendentes.

## File access and delegation

O worktree fica no estado do usuário, fora da pasta do projeto. Use os caminhos
absolutos devolvidos pela preparação para leitura e edição. No Claude Code,
`/add-dir <caminho-do-worktree>` permite adicionar o diretório à sessão. Os
insumos também precisam estar acessíveis para leitura.

`allowed-tools` declara as ferramentas permitidas durante a invocação da skill;
as permissões de diretórios da sessão continuam valendo. Conceda acesso aos
caminhos apresentados quando necessário.

Cada arquivo delegado tem um responsável por rodada, com conteúdo atual,
propostas, decisões e contexto entre páginas. O principal espera os delegados,
revê o conjunto e conduz Git, limpeza e publicação. O estado de preparação
pertence ao processo MCP; preserve o caminho para consultar os arquivos após
uma interrupção.

## MCP tools

O servidor expõe oito ferramentas:

- `memory_search`: trechos de texto com caminho, linhas e revisão Git.
- `memory_read`: arquivo ou janela de linhas na revisão publicada.
- `memory_proposal_list`: propostas por caminho/blob e categorias de `gpm.json`.
- `memory_propose`: captura de Markdown livre na branch de propostas.
- `memory_promote_prepare`: worktree e insumos fixos de todas ou algumas propostas.
- `memory_promote_check`: diff atual, somente leitura.
- `memory_promote_submit`: commit dos caminhos revisados, envio e PR.
- `memory_promote_finish`: confirmação de merge e limpeza correspondente.

Consulte parâmetros e limites em [funcionalidades](../../docs/features.md).
Falhas são informadas com o estado conhecido e conservam os arquivos preparados.
Se uma operação remota tiver resultado incerto, confira o estado antes de repeti-la.
