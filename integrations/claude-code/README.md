# Claude Code

A integração usa MCP por stdio e as skills explícitas `/remember` e `/promote`.
Não instala hooks nem captura sessões automaticamente.

## Setup

Depois de instalar a CLI, execute por máquina:

```sh
gpm setup git@github.com:org/memoria.git --categories domains,decision
```

O setup prepara o clone bare e os worktrees `read` e `proposals` no diretório de
estado do usuário, salva a URL, registra `gpm mcp` no escopo `user` do Claude
Code e instala `~/.claude/skills/{remember,promote}/SKILL.md`. Repita o setup
para reparar a instalação ou atualizar o caminho do binário. Consulte o
[guia do fluxo](../../docs/workflow.md) para requisitos e configuração do remoto.

A leitura usa apenas `memories/<categoria-configurada>/**/*.md` da `main`.
Propostas em `proposals/<id>-<slug>.md` ficam fora da busca cotidiana.

## Capture

Em uma sessão do Claude Code, invoque:

```text
/remember <fato>
```

O agente pesquisa memórias e propostas para sugerir ação e destino, apresenta o
texto que será compartilhado e espera sua confirmação. A proposta tem `title`
e `content` livres; ação, destino e origem relevante são texto. A captura cria
a proposta na branch remota `proposals` pela API do GitHub. Exige rede e `gh`
autenticado; uma falha é mostrada e interrompe a tentativa. A `main` não muda.

## Consolidate

Invoque explicitamente:

```text
/promote
```

O agente prepara um worktree separado a partir da `main` e recebe cópias de
todas as propostas de uma revisão fixa, fora do worktree. Leia e discuta com o
agente o plano global na conversa: grupos, destinos e justificativas para cada
proposta. Ele lê integralmente todas as propostas e memórias antes de propor o
plano. Se não couberem no contexto, informa o limite e para.

Aprove o plano antes da escrita. O agente prepara diretamente os arquivos
finais, pode delegar arquivos independentes e revisa a coerência do conjunto.
Reorganizações ficam limitadas às memórias relacionadas ao plano, preservando
conhecimento válido. Mudanças materiais exigem nova decisão na conversa.

A conferência completa IDs de headings novos, valida formato e categorias e
produz o diff atual. O agente mostra resumo global, diff por memória e caminho
do worktree. Você pode abrir esse caminho no VS Code e editar os mesmos
arquivos; o editor não abre automaticamente. Durante a edição humana, os
agentes suspendem a escrita e não executam a conferência. Diga na conversa
quando terminar ou quais ajustes deseja. Ajustes partem dos arquivos atuais e
preservam suas edições fora do pedido.

Aprove a publicação dos arquivos e do diff atuais. Se os arquivos mudarem após
a revisão, o agente deve apresentar a mudança e obter nova aprovação. O envio
cria um commit global contendo somente memórias e abre um PR para `main`, com
resumo dos destinos ou justificativas de todas as propostas. O merge é humano.
As propostas permanecem pendentes até a confirmação do merge.

Depois do merge, na máquina que publicou, execute:

```sh
gpm promote --finish
```

Você também pode pedir `memory_promote_finish` ou invocar o próximo `/promote`.
A conclusão consulta o GitHub, confirma o commit na `main`, remove somente
propostas cujo caminho e conteúdo ainda correspondam aos insumos concluídos e
atualiza a leitura local. Propostas novas ou alteradas permanecem pendentes.
O registro local vincula PR e insumos para essa limpeza; não restaura uma sessão
de edição. Esta versão não inclui retomada, recuperação automática ou
coordenação entre promoções. Um diff canônico vazio não publica nem limpa.

## File access and delegation

O worktree de promoção fica no estado do usuário, fora da pasta do projeto.
Use o caminho absoluto devolvido pela preparação para leitura e edição. Se a
sessão pedir acesso, autorize esse diretório pelas permissões normais do host;
no Claude Code você pode adicionar a pasta com `/add-dir <caminho-do-worktree>`.
Os caminhos dos insumos também precisam estar acessíveis para leitura.

A declaração `allowed-tools` da skill permite usar as ferramentas indicadas
durante a invocação. Ela não restringe todo o conjunto de ferramentas da sessão
nem concede acesso permanente a diretórios. As permissões do host continuam
valendo. Consulte a [referência de skills do Claude Code](https://code.claude.com/docs/en/skills)
e a [referência de ferramentas](https://code.claude.com/docs/en/tools-reference).

O principal pode delegar memórias independentes após a aprovação do plano.
Cada arquivo tem um responsável por rodada, com conteúdo atual, propostas,
decisões e contexto entre páginas. O principal espera os delegados terminarem,
revê o conjunto e mantém Git, limpeza e publicação sob sua responsabilidade.
Veja a [documentação de subagentes](https://code.claude.com/docs/en/sub-agents).

## MCP tools

O servidor expõe nove ferramentas:

- `memory_search`, `memory_expand` e `memory_page_read`: leitura da `main`.
- `memory_propose` e `memory_proposal_list`: captura e consulta de propostas.
- `memory_promote_prepare`: worktree e inventários fixos.
- `memory_promote_check`: formato canônico e diff dos arquivos atuais.
- `memory_promote_submit`: commit global, envio e PR após aprovação.
- `memory_promote_finish`: confirmação de merge e limpeza correspondente.

A [referência de funcionalidades](../../docs/features.md) detalha parâmetros e
limites. Se o MCP estiver ausente ou uma ferramenta falhar, o agente mostra a
falha e para, conservando os arquivos preparados. A integração não modifica a
configuração global de permissões para liberar diretórios automaticamente.
