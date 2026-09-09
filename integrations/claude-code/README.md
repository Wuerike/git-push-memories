# Claude Code

Esta integracao usa MCP sobre stdio e o skill explicito `/remember`. Ela nao instala hooks, nao captura sessoes e nao concede ao Claude Code autoridade para apply ou publish.

## Configurar

Depois de instalar a CLI, execute uma vez:

```bash
gpm setup /absolute/path/memory-repository
```

O repositorio deve ser um worktree Git ja clonado e possuir pelo menos um commit. O comando:

- salva o memory root na configuracao global do usuario;
- deriva um state root privado em `$XDG_STATE_HOME/git-push-memories` ou `~/.local/state/git-push-memories`;
- registra `gpm mcp` no escopo `user` do Claude Code;
- instala o skill global em `~/.claude/skills/remember/SKILL.md`.

O setup e idempotente. Execute-o novamente depois de mover o binario ou para selecionar outro repositorio de memoria.

O repositorio usa dois namespaces versionaveis:

```text
memories/*.md       memoria canonica pesquisavel
proposals/*.json    propostas em branches gpm/proposal/<id>
```

Arquivos Markdown fora de `memories/`, incluindo `README.md`, nao participam da recuperacao. Paginas dentro de subdiretorios de `memories/` tambem nao sao consideradas.

## Usar

Em qualquer sessao do Claude Code:

```text
/remember <fato>
```

O fluxo pesquisa a memoria, expande ate tres candidatos, oferece somente operacoes aplicaveis e exige confirmacao antes de criar uma proposta. A proposta aprovada pelo Intake Guard e commitada como `proposals/<id>.json` em uma branch local dedicada sem alterar o checkout atual. Busca ambigua, cancelamento, budget insuficiente ou falha MCP encerram o fluxo sem persistencia alternativa.

O servidor expoe somente:

- `memory_search`;
- `memory_expand`;
- `memory_propose_create_page`;
- `memory_propose_create_section`;
- `memory_propose_replace`.

Apply e publish continuam disponiveis somente na CLI:

```bash
gpm proposal show <proposal-id>
git -C <memory-repository> push -u origin gpm/proposal/<proposal-id>
gpm proposal apply <proposal-id> --revision <revision>
gpm proposal publish <proposal-id>
```

O push da branch de proposta e opcional e sempre humano; o produto nao acessa remotos. A branch preserva a proposta para backup ou revisao, mas nao deve ser mesclada diretamente na branch corrente. Apply e publish continuam operando pelo ID da proposta.

Se o servidor MCP estiver ausente ou falhar, Claude Code deve mostrar a falha e encerrar a tentativa. Nenhuma configuracao de `SessionStart`, `Stop`, `PostToolUse` ou outro hook faz parte desta integracao.
