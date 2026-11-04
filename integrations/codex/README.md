# Codex

A integração oferece MCP por stdio e as skills explícitas `$remember` e
`$promote` para capturar e consolidar conhecimento compartilhado no Codex local.

## Setup

Instale `gpm`, Git e a CLI do Codex. Configure a identidade Git e as credenciais
do repositório de memória. Execute por máquina:

```sh
gpm setup git@github.com:org/memoria.git --agent codex --categories domains,decision
```

O setup prepara o clone bare e os worktrees `read` e `proposals`, salva a URL,
registra `gpm mcp` por `codex mcp add` e instala:

```text
~/.agents/skills/remember/SKILL.md
~/.agents/skills/remember/agents/openai.yaml
~/.agents/skills/promote/SKILL.md
~/.agents/skills/promote/agents/openai.yaml
```

Os arquivos `openai.yaml` configuram `allow_implicit_invocation: false`.
As skills são carregadas quando você as invoca explicitamente. A instalação
preserva arquivos de skills que não sejam gerenciados pelo GPM e informa o
conflito para você resolver.

A CLI do Codex mantém o servidor em `~/.codex/config.toml`, ou em
`$CODEX_HOME/config.toml` quando essa variável está definida. As skills do usuário
ficam em `~/.agents/skills`. Repita o setup para atualizar os arquivos gerenciados
e o caminho absoluto do binário. Outras configurações do Codex são preservadas.

Quando definidos no setup, `XDG_CONFIG_HOME`, `XDG_STATE_HOME` e `GH_CONFIG_DIR`
são registrados como diretórios do processo MCP. Execute novamente o setup se
mudar esses caminhos.

Confira o registro e, na sessão, a conexão:

```sh
codex mcp get git-push-memories
codex mcp list
```

Use `/mcp` dentro do Codex para conferir as ferramentas disponíveis. Reinicie a
sessão se a configuração ou as skills ainda não aparecerem. A consulta do
registro pelo setup verifica a configuração; a conexão MCP é aberta pelo Codex.

Captura e publicação exigem GitHub, rede e `gh` autenticado com acesso de escrita:

```sh
gh auth login
```

O processo `gpm mcp` precisa encontrar Git, `gh` e as credenciais no ambiente
em que o Codex o executa. Os requisitos do repositório estão no
[guia do fluxo](../../docs/workflow.md).

Se a autenticação depender de um agente SSH ou de tokens em variáveis de
ambiente, configure o encaminhamento na tabela existente do servidor no
`config.toml` do Codex:

```toml
[mcp_servers.git-push-memories]
# Preserve command, args e env registrados pelo setup.
env_vars = ["SSH_AUTH_SOCK", "GH_TOKEN", "GITHUB_TOKEN"]
```

Defina essas variáveis no ambiente que inicia o Codex. A lista encaminha os
valores em tempo de execução; mantenha os tokens fora dos arquivos de
configuração. Ao repetir o setup, confira as opções adicionais dessa tabela.

## Acesso aos arquivos

A consolidação lê insumos e edita arquivos fora do repositório de código.
O setup informa o diretório de estado desta memória. Para permitir escrita
nesse diretório em uma sessão com sandbox, inicie o Codex com esse caminho:

```sh
codex --add-dir "/caminho/absoluto/informado/pelo/setup"
```

As skills usam os caminhos absolutos retornados por `memory_promote_prepare`.
O diretório liberado deve abranger o worktree e os insumos. O setup registra
a integração; o acesso de escrita da sessão é configurado por você.

## Captura

Na conversa com o Codex, invoque:

```text
$remember O timeout do checkout é de cinco segundos.
```

O agente consulta memórias e propostas relacionadas, explica sua interpretação
e apresenta os campos exatos `title` e `content`. Confirme o texto para autorizar
o envio ao repositório compartilhado. A captura cria Markdown na branch
`proposals` e retorna caminho e commit. Segredos, dados pessoais privados e
material de sessão ficam fora da proposta.

## Consolidação

Invoque com todas as propostas, um assunto ou caminhos específicos:

```text
$promote
$promote regras de cobrança
$promote proposals/billing.md proposals/ownership.md
```

O agente prepara o worktree, lê os insumos e as memórias relacionadas e discute
o plano global. Aprove o plano antes da edição. Os destinos usam as categorias
de `gpm.json`. Revise o resumo e o diff completo; você também pode editar os
mesmos arquivos no VS Code. Durante sua edição, o agente suspende a escrita.

Aprove a publicação dos arquivos e do diff atuais para criar o commit, enviar
a branch e abrir o PR. Faça o merge no GitHub. Na máquina que publicou, conclua
com o próximo `$promote`, com `memory_promote_finish` ou pela CLI:

```sh
gpm promote --finish
```

A limpeza remove somente propostas selecionadas que mantêm o mesmo conteúdo.
Execute uma consolidação por vez, inclusive quando alternar entre agentes.
O estado preparado pertence ao processo MCP; conserve o caminho dos arquivos
para consultá-los após uma interrupção.

## Consulta durante o trabalho

Peça ao Codex para consultar a memória compartilhada quando precisar de regras
ou decisões de domínio. `memory_search` retorna caminhos, linhas e uma revisão
Git; `memory_read` permite aprofundar a leitura nessa revisão. As
[oito ferramentas MCP](../../docs/features.md#mcp-tools) ficam disponíveis
independentemente da invocação de uma skill.

Para orientar consultas em um projeto, você pode incluir no seu `AGENTS.md`:

```markdown
Quando a tarefa depender de regras ou decisões de domínio, consulte a memória
compartilhada com memory_search e aprofunde os resultados com memory_read,
usando a revisão Git retornada pela busca.
```

## Uso com Claude Code

As integrações compartilham o repositório de memória e os worktrees da máquina.
Para configurar Claude Code, execute o setup com a mesma URL e
`--agent claude-code`, conforme o [guia da integração](../claude-code/README.md).
O agente padrão de `gpm setup` é `claude-code`. Cada execução instala a integração
selecionada e preserva a outra.

Referências oficiais: [MCP no Codex](https://learn.chatgpt.com/docs/extend/mcp?surface=cli)
e [skills do Codex](https://learn.chatgpt.com/docs/build-skills).
