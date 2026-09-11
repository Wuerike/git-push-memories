# Estado das funcionalidades

Atualizado em 2026-09-11.

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
| Fase 3: curadoria e confianca | **Em andamento.** | T1-T7 entregam o lifecycle tecnico, o [E2E de T8](phase-3/t8-technical-e2e.md) fecha a Fase 3B e o [protocolo T9](phase-3/t9-value-evaluation-protocol.md) congela a avaliacao; execucao e decisao de valor continuam pendentes em T10. |

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
| `memory_reference_snapshot` | **Implementado.** | Produz referencias imutaveis a blobs commitados e vincula transitoriamente um checkout local, sem retornar ou persistir seu root. |
| `memory_reference_expand` | **Implementado.** | Retorna linhas limitadas do blob UTF-8 identificado para revisao sem persistir o conteudo. |
| `memory_propose` | **Implementado.** | Valida, grava e commita uma proposta Markdown; payload canonico byte-identico retorna a proposta existente com `duplicate: true` sem novo commit. |
| `memory_proposal_list` | **Implementado.** | Revalida schema e privacidade antes de listar propostas commitadas e categorias configuradas, sem criar estado privado ou alterar o repositorio. |
| `memory_curate_preview` | **Implementado.** | Gera sem escrita o diff e evidencia separada em candidatos canonicos e propostas relacionadas, no mesmo `HEAD`. |
| `memory_curate_apply` | **Implementado para destinos individuais.** | Consome uma vez um preview do mesmo processo, revalida drift e cria o commit exato de consumo ou descarte. |
| `memory_curate_batch_preview` | **Implementado.** | Revisa de uma a dezesseis decisoes independentes com proposal IDs e targets distintos; `keep_pending` preserva uma selecao ainda nao resolvida quando o lote possui outra decisao. |
| `memory_curate_batch_apply` | **Implementado.** | Revalida o lote e publica todas as decisoes em um unico commit recuperavel. |
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
| Schema fechado de proposta | **Implementado.** A entrada aceita somente `title`, `suggested_action`, `target` opcional, `knowledge`, `source_paths` e `source_references` opcional. |
| Acoes sugeridas | **Implementado.** Os valores aceitos sao `create`, `update` e `remove`; eles orientam a curadoria futura e nao alteram memoria por conta propria. |
| Target opcional | **Implementado.** Quando informado, deve ser um path Markdown dentro de uma categoria configurada. |
| Source paths v1 | **Legado.** Paths sao canonicalizados e examinados pela politica de privacidade, mas sempre aparecem como `legacy_unverified`. |
| Source references v2 | **Implementado.** Identificam lineage do repositorio, commit completo, path relativo e SHA-256 do blob; referencias e source paths nao vazios sao mutuamente exclusivos. |
| Intake Guard antes da persistencia | **Implementado.** Todo o payload e validado antes da criacao de lock, arquivo de proposta ou objeto Git. |
| Deteccao normativa | **Implementado para a politica v1.** Bloqueia padroes conhecidos de chaves, tokens, autorizacao, email, telefone, CPF e referencias de sessao. |
| Privacidade semantica | **Parcial.** Texto livre nao e classificado semanticamente como prompt, resposta ou transcript quando nao corresponde aos padroes normativos. |
| Limites | **Implementado.** Candidato, titulo, conhecimento, paths e quantidade de fontes possuem limites fechados. |
| Fail-closed da captura | **Implementado.** Schema, privacidade ou path invalidos encerram a chamada sem proposta. |
| Idempotencia da captura | **Implementada para igualdade exata.** O conteudo Markdown canonico completo e comparado sob lock; equivalencia semantica continua sendo evidencia para decisao humana. |

## Inbox e Git

| Funcionalidade | Estado atual |
|---|---|
| Formato da proposta | **Implementado.** Schema v1 preserva source paths legados; schema v2 serializa referencias estruturadas. Ambos mantem Markdown legivel, acao, target, titulo e conhecimento. |
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
| Snapshot de fontes no `/remember` | **Implementado.** A skill usa somente referencias retornadas pela ferramenta e nunca persiste o root local ou inventa hashes. |
| Dedupe no `/remember` | **Implementado.** Resposta `duplicate: true` informa que ID, path e commit existentes foram reutilizados sem nova publicacao. |
| `/curate` explicito | **Implementado para `create`, `update`, `remove` e descarte.** Pesquisa memoria, escolhe uma operacao, gera preview e exige confirmacao antes do apply. |
| Pesquisa antes da proposta | **Implementado no contrato.** O skill pesquisa uma vez e expande ate tres resultados antes de preparar a proposta. |
| Confirmacao humana | **Implementado no contrato.** Os campos exatos sao apresentados antes de uma unica chamada a `memory_propose`. |
| Fallback seguro | **Implementado no contrato e nos harnesses.** Cancelamento, budget insuficiente, MCP ausente ou erro encerram o fluxo sem escrita alternativa. |
| Captura automatica por hooks | **Adiado.** Nenhum hook de sessao e instalado. |

## Curadoria e governanca

| Funcionalidade | Estado atual |
|---|---|
| Listagem e selecao de propostas | **Implementado.** `/curate` lista o inbox e conduz a selecao de uma proposta. |
| Criacao de pagina canonica | **Implementado.** Uma proposta `create` pode gerar uma pagina nova dentro de uma categoria configurada. |
| Criacao de secao canonica | **Implementado.** Uma proposta `create` pode acrescentar uma secao H2 com ID derivado a uma pagina existente. |
| Atualizacao de secao canonica | **Implementado.** Uma proposta `update` pode substituir somente o corpo completo de uma secao folha identificada e vinculada ao content hash observado. |
| Remocao de memoria canonica | **Implementado para destinos individuais.** `remove_section` exclui uma secao folha identificada; `remove_page` aceita somente a forma exata de uma pagina gerada com uma unica secao `Overview`. Nao ha cascade delete ou reparo de links. |
| Consumo atomico de propostas | **Implementado para lotes limitados.** `create`, `update`, `remove` e descarte podem consumir de uma a dezesseis propostas em um unico commit; targets repetidos sao rejeitados. |
| Recovery de curadoria | **Implementado.** Marcador privado sem conteudo e indice reconciliado convergem para baseline antes de `update-ref` ou para o mesmo commit completo depois dele; lock estrangeiro nunca e removido. |
| Descarte auditavel | **Implementado.** Remove somente a proposta e registra decisao e reason code fechado no commit Git. |
| Duplicidade e contradicao | **Parcial e observavel.** Dedupe exato bloqueia publicacao repetida; preview separa candidatos canonicos e propostas relacionadas; `/curate` classifica equivalencia ou contradicao como avaliacao assistida e exige escolha humana. Nao ha detector semantico completo. |
| Integridade de referencias | **Implementado localmente.** Preview distingue `verified`, `unavailable`, `mismatch` e `legacy_unverified`; mudanca de status invalida apply. |
| Sustentacao semantica | **Decisao humana assistida.** `supports`, `contradicts` e `unclear` nao sao inferidos pelo verificador nem confundidos com integridade. |
| Validacao por agentes | **Adiado.** Nenhum agente possui autoridade de curadoria. |
| Pull request e politica de merge | **Adiado.** Revisao e push atuais usam Git comum, sem automacao remota. |
| Nova avaliacao de valor | **Pendente.** Os experimentos da Fase 2 avaliaram o lifecycle supersedido e nao foram repetidos com o inbox atual. |

## Verificacao atual

O E2E tecnico da Fase 3B usa o binario real e processos MCP separados para compor privacidade, referencias, dedupe, create, update, remove, discard, crash depois de `update-ref`, startup recovery e recuperacao final. `tests/e2e/run-phase-3.sh` produz evidencia sanitizada e executa as matrizes online-cache e offline.

`make verify` executa, em serie:

```text
go test ./...
go test -race ./...
go vet ./...
go build ./...
```

A suite atual cobre configuracao, categorias, parsing Markdown, busca e expand, politica de privacidade, commit e dedupe exato do inbox, referencias v1/v2, snapshot e verificacao local de blobs, evidencia relacionada, preview e apply individuais e em lote, fault injection e recovery, preservacao do indice, drift, concorrencia local, contratos de `/remember` e `/curate`, E2E tecnico por processos e recuperacao por usuario novo. Ainda nao existe experimento de valor para o fluxo introduzido pela ADR 0004.
