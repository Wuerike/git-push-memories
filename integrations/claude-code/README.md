# Claude Code

Esta integracao usa MCP sobre stdio e os skills explicitos `/remember` e `/curate`. Ela nao instala hooks, nao captura sessoes e nao altera memoria canonica durante a captura.

## Configurar

Depois de instalar a CLI, execute uma vez:

```bash
gpm setup /absolute/path/memory-repository
```

O repositorio deve ser um worktree Git ja clonado, possuir pelo menos um commit e conter um `gpm.json` valido com `memory_categories`. A criacao de propostas exige que o checkout esteja exatamente em `main`. O comando:

- salva o memory root na configuracao global do usuario;
- deriva um state root privado em `$XDG_STATE_HOME/git-push-memories` ou `~/.local/state/git-push-memories`;
- registra `gpm mcp` no escopo `user` do Claude Code;
- instala os skills globais em `~/.claude/skills/remember/SKILL.md` e `~/.claude/skills/curate/SKILL.md`.

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

O fluxo pesquisa a memoria, expande ate tres candidatos, sugere `create`, `update` ou `remove` e exige confirmacao antes de criar uma proposta. A proposta aprovada pelo Intake Guard e commitada como Markdown diretamente em `main`; memoria canonica e staging alheio permanecem inalterados. Busca ambigua, cancelamento, budget insuficiente ou falha MCP encerram o fluxo sem persistencia alternativa.

Para transformar uma proposta de criacao em memoria canonica:

```text
/curate [proposal-id]
```

A skill lista o inbox depois de revalidar sua privacidade, seleciona uma proposta `create`, mostra seu conteudo e pesquisa memoria relacionada. Quando uma pagina existente deve receber o conhecimento, escolhe `create_section` e um heading; caso contrario escolhe `create_page` e um path novo. Somente depois de mostrar e confirmar o diff exato ela aplica o preview. O commit resultante altera a memoria canonica e remove a proposta consumida, sem incluir staging alheio. O preview e single-use e qualquer drift exige uma nova invocacao explicita. `update`, `remove`, descarte e lotes ainda nao sao suportados.

O servidor expoe:

- `memory_search`;
- `memory_expand`;
- `memory_propose`.
- `memory_proposal_list`;
- `memory_curate_preview`;
- `memory_curate_apply`.

O commit local pode ser revisado e enviado com Git comum:

```bash
git -C <memory-repository> show --stat HEAD
git -C <memory-repository> push
```

O push e sempre humano e o produto nao acessa remotos.

Se o servidor MCP estiver ausente ou falhar, Claude Code deve mostrar a falha e encerrar a tentativa. Nenhuma configuracao de `SessionStart`, `Stop`, `PostToolUse` ou outro hook faz parte desta integracao.
