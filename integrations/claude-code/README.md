# Claude Code

Esta integracao usa MCP sobre stdio e os skills explicitos `/remember` e `/promote`. Ela nao instala hooks, nao captura sessoes e nao altera memoria canonica durante a captura.

## Configurar

Depois de instalar a CLI, execute uma vez:

```bash
gpm setup /absolute/path/memory-repository
```

O repositorio deve ser um worktree Git ja clonado, possuir pelo menos um commit e conter um `gpm.json` valido com `memory_categories`. A criacao de propostas exige que o checkout esteja exatamente em `main`. O comando:

- salva o memory root na configuracao global do usuario;
- deriva um state root privado em `$XDG_STATE_HOME/git-push-memories` ou `~/.local/state/git-push-memories`;
- registra `gpm mcp` no escopo `user` do Claude Code;
- instala os skills globais em `~/.claude/skills/remember/SKILL.md` e `~/.claude/skills/promote/SKILL.md`.

O setup e idempotente. Execute-o novamente depois de mover o binario ou para selecionar outro repositorio de memoria.

O repositorio usa dois namespaces versionaveis:

```text
memories/<categoria-configurada>/**/*.md    memoria canonica pesquisavel
proposals/<id>-<slug>.md                    inbox de propostas
```

Markdown fora das categorias configuradas, incluindo `README.md`, arquivos diretos em `memories/` e o inbox `proposals/`, nao participa da recuperacao.

## Usar

Em qualquer sessao do Claude Code:

```text
/remember <fato>
```

O fluxo pesquisa a memoria, expande ate tres candidatos, sugere `create`, `update` ou `remove` e exige confirmacao antes de criar uma proposta. Fontes commitadas passam por `memory_reference_snapshot`; somente repository lineage, revisao completa, path relativo e hash do blob entram na proposta v2, nunca o root local. Uma proposta byte-identica existente e retornada com `duplicate: true` sem novo commit.

Para consumir ou descartar uma proposta:

```text
/promote [proposal-id ...]
```

A skill lista o inbox depois de revalidar sua privacidade, mostra de uma a dezesseis propostas selecionadas e pesquisa memoria relacionada. Cada target deve ser distinto. O preview apresenta candidatos canonicos, propostas relacionadas e referencia por referencia os estados `verified`, `unavailable`, `mismatch` ou `legacy_unverified`. Integridade nao implica que a fonte sustenta a afirmacao; a skill e a pessoa avaliam separadamente `supports`, `contradicts` ou `unclear`. O diff exato ainda exige confirmacao, staging alheio e preservado e qualquer mudanca de evidencia invalida o preview single-use.

O servidor expoe:

- `memory_search`;
- `memory_expand`;
- `memory_reference_snapshot`;
- `memory_reference_expand`;
- `memory_page_read`;
- `memory_propose`;
- `memory_proposal_list`;
- `memory_promote_preview`;
- `memory_promote_apply`;
- `memory_promote_batch_preview`;
- `memory_promote_batch_apply`.

O commit local pode ser revisado e enviado com Git comum:

```bash
git -C <memory-repository> show --stat HEAD
git -C <memory-repository> push
```

O push e sempre humano e o produto nao acessa remotos.

Se o servidor MCP estiver ausente ou falhar, Claude Code deve mostrar a falha e encerrar a tentativa. Nenhuma configuracao de `SessionStart`, `Stop`, `PostToolUse` ou outro hook faz parte desta integracao.
