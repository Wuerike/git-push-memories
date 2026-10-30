# Claude Code

Esta integracao usa MCP sobre stdio e os skills explicitos `/remember` e `/promote`. Ela nao instala hooks, nao captura sessoes e nao altera memoria canonica durante a captura.

## Configurar

Depois de instalar a CLI, execute uma vez por maquina:

```bash
gpm setup git@github.com:org/memoria.git --categories domains,decision
```

O repositorio e remoto. Vazio, o setup cria `gpm.json`, a `main` e a `proposals`; com `gpm.json`, so clona e valida. O comando:

- mantem um clone bare gerenciado em `$XDG_STATE_HOME/git-push-memories/<id>` (ou `~/.local/state/...`), com worktrees `read` (`main`), `proposals` e `release`;
- salva a URL na configuracao global do usuario;
- registra `gpm mcp` no escopo `user` do Claude Code;
- instala os skills globais em `~/.claude/skills/remember/SKILL.md` e `~/.claude/skills/promote/SKILL.md`.

O setup e idempotente: repita-o para reparar ou depois de mover o binario. Trocar de repositorio exige remover o diretorio de estado.

O repositorio usa dois namespaces versionaveis:

```text
memories/<categoria-configurada>/**/*.md    memoria canonica pesquisavel (main)
proposals/<id>-<slug>.md                    inbox de propostas (branch proposals)
```

Markdown fora das categorias configuradas, incluindo `README.md`, arquivos diretos em `memories/` e `proposals/`, nao participa da recuperacao.

## Usar

Em qualquer sessao do Claude Code:

```text
/remember <fato>
```

O fluxo pesquisa a memoria, expande ate tres candidatos, sugere `create`, `update` ou `remove` e exige confirmacao antes de criar uma proposta. A proposta e texto livre (`title` e `content`): acao sugerida, destino e origem, se importarem, vao escritos no texto. Ela e criada na `proposals` remota pela API do GitHub (`gh` autenticado e rede sao requisitos); se falhar, nada e gravado e o `/remember` e repetido.

Para consolidar as propostas pendentes em memoria canonica:

```text
/promote
```

A skill abre (ou retoma) o release, agrupa todas as propostas por assunto ou pagina, mostra plano, diff e evidencias e aplica lotes de ate dezesseis decisoes, cada um confirmado pela pessoa, ate nao restar nenhuma proposta. Entao envia o release para revisao: push da branch e pull request para a `main` (via `gh`). O merge e humano; o proximo `/promote` ou `gpm promote --finish` finaliza o release. O preview apresenta candidatos canonicos e propostas relacionadas.

Release abandonado: `gpm promote --resume` (continua; cria outro PR se o anterior foi fechado) ou `gpm promote --abort`.

O servidor expoe:

- `memory_search`, `memory_expand`, `memory_page_read` (aceitam `source: release`);
- `memory_propose`, `memory_proposal_list`;
- `memory_promote_open`, `memory_promote_status`, `memory_promote_submit`;
- `memory_promote_preview`, `memory_promote_apply`, `memory_promote_batch_preview`, `memory_promote_batch_apply`.

O envio ao remote e automatico no `/remember` (aviso na confirmacao) e no `/promote`; ninguem precisa rodar `git push`.

Se o servidor MCP estiver ausente ou falhar, Claude Code deve mostrar a falha e encerrar a tentativa. Nenhuma configuracao de `SessionStart`, `Stop`, `PostToolUse` ou outro hook faz parte desta integracao.
