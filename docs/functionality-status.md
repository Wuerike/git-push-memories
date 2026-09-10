# Estado das funcionalidades

Atualizado em 2026-09-10.

Este documento descreve o comportamento exposto pela implementacao atual. Os documentos em `docs/phase-1/` e `docs/phase-2/` preservam decisoes, protocolos e evidencias dos fluxos avaliados naquelas fases; eles nao substituem este inventario operacional.

Os estados usados sao:

- **implementado**: comportamento disponivel e coberto pela verificacao atual;
- **parcial**: existe um fluxo utilizavel, mas falta parte relevante do resultado;
- **adiado**: capacidade deliberadamente reservada para uma etapa posterior;
- **removido**: comportamento que existiu no produto, mas nao faz parte da interface atual.

## Estado das fases

| Fase | Estado atual | Observacao |
|---|---|---|
| Fase 1: avaliacao de projetos | **Concluida.** | A [ADR 0001](adr/0001-implementation-base.md) escolheu uma implementacao propria em Go com adaptacao seletiva do motor Markdown do agent-memory. |
| Fase 2: fluxo vertical minimo | **Concluida como evidencia historica, com decisao `revisar o fluxo`.** | O lifecycle avaliado foi posteriormente substituido; seus resultados nao validam automaticamente o fluxo atual. |
| Transicao pos-Fase 2 | **Concluida.** | A [ADR 0004](adr/0004-markdown-proposal-inbox-and-memory-categories.md) introduziu categorias configuradas e o inbox Markdown em `main`. |
| Fase 3: curadoria e confianca | **Em andamento.** | A [T1](phase-3/t1-create-page-curation.md) transforma uma proposta `create` em pagina canonica; as demais operacoes e garantias do [plano da Fase 3](phase-3/plan.md) ainda estao pendentes. |

## Superficie operacional

### CLI

| Comando | Estado | Comportamento atual |
|---|---|---|
| `gpm setup <memory-root>` | **Implementado.** | Valida e salva o repositorio de memoria, registra `gpm mcp` no Claude Code e instala o skill global `/remember`. |
| `gpm mcp` | **Implementado.** | Executa o servidor MCP transitorio sobre stdio. |
| `gpm retrieve <query> --budget-bytes <n>` | **Implementado.** | Executa a mesma recuperacao textual usada por `memory_search`. |
| `gpm version` e `gpm help` | **Implementado.** | Funcionam sem configuracao previa do repositorio de memoria. |
| `gpm proposal show/reject/apply/publish` | **Removido.** | Nao existe lifecycle de proposta paralelo ao Git. Propostas sao arquivos Markdown comuns no inbox. |

### MCP

| Tool | Estado | Comportamento atual |
|---|---|---|
| `memory_search` | **Implementado.** | Busca secoes folha da memoria canonica dentro de um budget exato. |
| `memory_expand` | **Implementado.** | Expande um handle de busca e rejeita handles obsoletos. |
| `memory_propose` | **Implementado.** | Valida, grava e commita uma proposta Markdown no inbox depois da confirmacao conduzida pelo cliente. |
| `memory_proposal_list` | **Implementado.** | Revalida schema e privacidade antes de listar propostas commitadas e categorias configuradas, sem criar estado privado ou alterar o repositorio. |
| `memory_curate_preview` | **Implementado para `create`.** | Valida uma proposta, recupera memoria canonica relacionada e gera o diff de criacao de pagina e remocao da proposta sem escrever. |
| `memory_curate_apply` | **Implementado para `create`.** | Consome uma vez um preview do mesmo processo, revalida drift e cria o commit de curadoria. |
| Tools legadas `memory_propose_create_*` e `proposal_apply/publish` | **Removido.** | O lifecycle anterior continua ausente; a curadoria atual usa preview single-use e um commit do inbox para o canônico. |

## Configuracao e layout

| Funcionalidade | Estado atual |
|---|---|
| Repositorio local explicito | **Implementado.** O memory root deve ser a raiz de um worktree Git existente e possuir `HEAD`. Nao ha descoberta implicita pelo diretorio atual. |
| Configuracao global | **Implementado.** `setup` salva o memory root na configuracao do usuario para uso por sessoes em outros repositorios de codigo. |
| `gpm.json` obrigatorio | **Implementado.** A raiz deve declarar `schema_version: 1` e `memory_categories`. Campos desconhecidos, schema invalido ou conteudo adicional sao rejeitados. |
| Categorias | **Implementado.** A lista deve ser nao vazia, sem duplicatas e composta por nomes lowercase kebab-case. |
| Namespace canonico | **Implementado.** Somente `memories/<categoria-configurada>/**/*.md` participa da recuperacao. |
| Exclusoes da recuperacao | **Implementado.** Markdown na raiz, arquivos diretos em `memories/`, categorias nao configuradas e `proposals/` sao ignorados. |
| Inbox | **Implementado.** Propostas usam `proposals/<id>-<slug>.md`. |
| Estado privado | **Implementado.** Locks ficam no state root derivado por repositorio e fora de worktrees Git. |

## Markdown e recuperacao

| Funcionalidade | Estado atual |
|---|---|
| Schema de memoria | **Implementado para leitura.** Documentos exigem UTF-8 valido, headings com `<!-- @id: slug -->`, IDs validos e unicos e ausencia de conflitos de merge nao resolvidos. |
| Identidade de secao | **Implementado.** Handles usam path canonico, section ID e hash do conteudo. |
| Busca textual | **Implementado.** O ranking conta termos distintos da query presentes no path, heading ou corpo e usa ordenacao deterministica para empates. |
| Selecao de secoes | **Implementado.** A busca retorna apenas secoes folha; secoes estruturais com filhos nao competem como resultados independentes. |
| Busca recursiva por categoria | **Implementado.** Subdiretorios abaixo de cada categoria configurada sao percorridos. |
| Escopo por path | **Implementado.** Um prefixo relativo canonico pode restringir a busca; paths absolutos e escapes por `..` sao rejeitados. |
| Budget | **Implementado.** O JSON retornado nunca excede `budget_bytes`; budget insuficiente retorna erro tipado. |
| Expansao e obsolescencia | **Implementado.** `memory_expand` confirma section ID e content hash no estado atual antes de retornar o corpo. |
| Leitura sem mutacao | **Implementado.** Search e expand nao criam state root, indice, proposal ou alteracao Git. |
| Busca semantica, FTS ou vetorial | **Adiado.** Nao existe indice persistente, banco vetorial ou grafo. |

## Privacidade e captura

| Funcionalidade | Estado atual |
|---|---|
| Schema fechado de proposta | **Implementado.** A entrada aceita somente `title`, `suggested_action`, `target` opcional, `knowledge` e `source_paths`. |
| Acoes sugeridas | **Implementado.** Os valores aceitos sao `create`, `update` e `remove`; eles orientam a curadoria futura e nao alteram memoria por conta propria. |
| Target opcional | **Implementado.** Quando informado, deve ser um path Markdown dentro de uma categoria configurada. |
| Source paths | **Parcial.** Paths sao canonicalizados e examinados pela politica de privacidade, mas existencia e aderencia ao conhecimento nao sao verificadas. |
| Intake Guard antes da persistencia | **Implementado.** Todo o payload e validado antes da criacao de lock, arquivo de proposta ou objeto Git. |
| Deteccao normativa | **Implementado para a politica v1.** Bloqueia padroes conhecidos de chaves, tokens, autorizacao, email, telefone, CPF e referencias de sessao. |
| Privacidade semantica | **Parcial.** Texto livre nao e classificado semanticamente como prompt, resposta ou transcript quando nao corresponde aos padroes normativos. |
| Limites | **Implementado.** Candidato, titulo, conhecimento, paths e quantidade de fontes possuem limites fechados. |
| Fail-closed da captura | **Implementado.** Schema, privacidade ou path invalidos encerram a chamada sem proposta. |
| Idempotencia da captura | **Nao implementada.** Cada chamada aceita cria uma nova proposta; evitar duplicatas depende do `/remember` e da curadoria futura. |

## Inbox e Git

| Funcionalidade | Estado atual |
|---|---|
| Formato da proposta | **Implementado.** Markdown legivel com frontmatter `schema_version: 1`, acao sugerida, target opcional e source paths, seguido por titulo e conhecimento. |
| Identidade | **Implementado.** Cada proposta recebe ID aleatorio hexadecimal e slug derivado do titulo. |
| Branch obrigatoria | **Implementado.** A captura exige que o checkout esteja exatamente em `main`. |
| Commit path-scoped | **Implementado.** O commit contem exatamente o novo arquivo de proposta e parte do `HEAD` mais recente. |
| Preservacao de staging | **Implementado.** Alteracoes staged ou unstaged fora do path da proposta nao sao consumidas nem descartadas. |
| Hooks Git | **Implementado como restricao.** O commit usa plumbing (`commit-tree` e `update-ref`) e nao executa hooks de commit. |
| Concorrencia no mesmo clone | **Implementado.** Um lock do inbox serializa sessoes e o index lock impede interferencia de operacoes Git concorrentes durante a publicacao local. |
| Protecao de `HEAD` | **Implementado.** Branch e parent sao revalidados e a atualizacao de `main` usa compare-and-swap. |
| Operacoes remotas | **Nao executadas.** O produto nao faz fetch, pull, push nem cria pull request. |
| Concorrencia entre clones | **Responsabilidade humana.** Um push non-fast-forward e resolvido pelo fluxo Git normal. |

## Integracao com Claude Code

| Funcionalidade | Estado atual |
|---|---|
| Setup global | **Implementado.** O MCP e `/remember` sao instalados no escopo do usuario. |
| MCP por stdio | **Implementado.** Nao ha daemon, porta local ou container obrigatorio. |
| `/remember` explicito | **Implementado.** O skill nao e invocado automaticamente pelo modelo. |
| `/curate` explicito | **Implementado para `create`.** Lista propostas, gera um preview e exige confirmacao antes de uma unica chamada de apply. |
| Pesquisa antes da proposta | **Implementado no contrato.** O skill pesquisa uma vez e expande ate tres resultados antes de preparar a proposta. |
| Confirmacao humana | **Implementado no contrato.** Os campos exatos sao apresentados antes de uma unica chamada a `memory_propose`. |
| Fallback seguro | **Implementado no contrato e nos harnesses.** Cancelamento, budget insuficiente, MCP ausente ou erro encerram o fluxo sem escrita alternativa. |
| Captura automatica por hooks | **Adiado.** Nenhum hook de sessao e instalado. |

## Curadoria e governanca

| Funcionalidade | Estado atual |
|---|---|
| Listagem e selecao de propostas | **Implementado.** `/curate` lista o inbox e conduz a selecao de uma proposta. |
| Criacao de pagina canonica | **Implementado.** Uma proposta `create` pode gerar uma pagina nova dentro de uma categoria configurada. |
| Atualizacao e remocao de memoria canonica | **Adiado.** Permanecem para a Fase 3B. |
| Consumo atomico de propostas | **Parcial.** O fluxo `create` adiciona a pagina e remove a proposta no mesmo commit; lotes ainda nao sao suportados. |
| Descarte auditavel | **Adiado.** Ainda nao existe fluxo proprio para rejeitar ou remover propostas. |
| Deteccao de duplicidade e contradicao | **Adiado.** Search orienta a captura, mas nao decide equivalencia ou conflito semantico. |
| Validacao por agentes | **Adiado.** Nenhum agente possui autoridade de curadoria. |
| Pull request e politica de merge | **Adiado.** Revisao e push atuais usam Git comum, sem automacao remota. |
| Nova avaliacao de valor | **Pendente.** Os experimentos da Fase 2 avaliaram o lifecycle supersedido e nao foram repetidos com o inbox atual. |

## Verificacao atual

`make verify` executa, em serie:

```text
go test ./...
go test -race ./...
go vet ./...
go build ./...
```

A suite atual cobre configuracao, categorias, parsing Markdown, busca e expand, politica de privacidade, commit do inbox, listagem privada do inbox, preview e apply de criacao, preservacao do indice, drift, concorrencia local, contratos de `/remember` e `/curate` e recuperacao da memoria curada por um novo processo. Nao existe atualmente um E2E classificatorio nem experimento de valor para o fluxo introduzido pela ADR 0004.
