# Claude Code

Esta integracao usa MCP sobre stdio e o skill explicito `/remember`. Ela nao instala hooks, nao captura sessoes e nao concede ao Claude Code autoridade para apply ou publish.

## Configurar

Depois de instalar a CLI, execute uma vez:

```bash
git-push-memories setup /absolute/path/memory-repository
```

O repositorio deve ser um worktree Git ja clonado e possuir pelo menos um commit. O comando:

- salva o memory root na configuracao global do usuario;
- deriva um state root privado em `$XDG_STATE_HOME/git-push-memories` ou `~/.local/state/git-push-memories`;
- registra `git-push-memories mcp` no escopo `user` do Claude Code;
- instala o skill global em `~/.claude/skills/remember/SKILL.md`.

O setup e idempotente. Execute-o novamente depois de mover o binario ou para selecionar outro repositorio de memoria.

## Usar

Em qualquer sessao do Claude Code:

```text
/remember <fato>
```

O fluxo pesquisa a memoria, expande ate tres candidatos, oferece somente operacoes aplicaveis e exige confirmacao antes de criar uma proposta. Busca ambigua, cancelamento, budget insuficiente ou falha MCP encerram o fluxo sem persistencia alternativa.

O servidor expoe somente:

- `memory_search`;
- `memory_expand`;
- `memory_propose_create_page`;
- `memory_propose_create_section`;
- `memory_propose_replace`.

Apply e publish continuam disponiveis somente na CLI:

```bash
git-push-memories proposal show <proposal-id>
git-push-memories proposal apply <proposal-id> --revision <revision>
git-push-memories proposal publish <proposal-id>
```

Se o servidor MCP estiver ausente ou falhar, Claude Code deve mostrar a falha e encerrar a tentativa. Nenhuma configuracao de `SessionStart`, `Stop`, `PostToolUse` ou outro hook faz parte desta integracao.
