# Claude Code

Esta integracao usa somente MCP sobre stdio e skills explicitos de proposta. Ela nao instala hooks, nao captura sessoes e nao concede ao Claude Code autoridade para apply ou publish.

## Configurar o servidor MCP

Compile `git-push-memories` e registre o processo local no projeto consumidor com paths absolutos:

```bash
claude mcp add --transport stdio --scope project git-push-memories -- \
  /absolute/path/git-push-memories mcp \
  --memory-root /absolute/path/memory-repository \
  --code-root /absolute/path/code-repository \
  --state-root /absolute/path/private-state
```

Memory root e code root devem ser worktrees Git distintos. State root deve ser privado, ficar fora dos dois repositorios e nao conter estado compartilhado ou dados de sessao.

O servidor registrado expoe somente:

- `memory_search`;
- `memory_expand`;
- `memory_propose_create_page`;
- `memory_propose_create_section`;
- `memory_propose_replace`.

Apply e publish continuam disponiveis somente na CLI para execucao humana.

## Habilitar a instrucao explicita

Instale o skill no code root do projeto consumidor:

```bash
install -D -m 0644 \
  /absolute/path/git-push-memories/integrations/claude-code/skills/propose-memory/SKILL.md \
  /absolute/path/code-repository/.claude/skills/propose-memory/SKILL.md
```

Instale da mesma forma `skills/propose-memory-page/SKILL.md` e `skills/propose-memory-section/SKILL.md` para disponibilizar as operacoes explicitas de baixo nivel. Cada skill recebe intencao e conteudo, deixa schema/anchors a cargo do produto e chama exatamente uma tool de proposta. O frontmatter limita cada skill a sua operacao.

`/propose-memory` continua sendo o caminho de replace compativel. `/propose-memory-page` e `/propose-memory-section` existem para uso explicito e diagnostico; o fluxo orientado por pesquisa sera fornecido por `/remember`.

O caminho principal e `skills/remember/SKILL.md`. Instale-o como `.claude/skills/remember/SKILL.md` e invoque `/remember <fato>`. Ele pesquisa primeiro, expande no maximo tres candidatos, oferece somente operacoes aplicaveis e exige confirmacao antes de criar uma proposta. Busca ambigua, cancelamento, budget insuficiente ou falha MCP encerram o fluxo sem persistencia alternativa.

Se o servidor MCP estiver ausente ou falhar, Claude Code deve mostrar a falha e encerrar a tentativa. Nao deve repetir a chamada, inferir o target de texto livre, nem gravar o candidato em arquivo, log, spool ou ferramenta alternativa.

Nenhuma configuracao de `SessionStart`, `Stop`, `PostToolUse` ou outro hook faz parte desta integracao.
