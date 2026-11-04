# Promote Simplification Specification

**Approved**: 2026-11-03 - user: "aprovado"
**Status:** Execução aprovada e em andamento; provas finais pendentes.
**Scope:** Large; fluxo principal entre preparação Git, skill, ferramentas de promoção e publicação GitHub.

## Problem Statement

O `/promote` atual representa uma consolidação global como decisões individuais, folds, lotes de até dezesseis, previews e transações. Esse contrato dificulta combinar propostas e revisar memórias finais com a pessoa. A mudança simplifica o percurso completo, usando o agente para organizar o conhecimento e arquivos comuns de um worktree para preparar e revisar o resultado.

## Goals

- Analisar todas as propostas iniciais e todas as memórias existentes antes de apresentar o plano global.
- Discutir o plano na conversa e preparar memórias finais somente após sua aprovação.
- Revisar os mesmos arquivos pelo terminal ou pelo editor, preservando edições humanas.
- Publicar um commit global por PR para `main` e limpar somente propostas concluídas depois do merge.
- Substituir a infraestrutura de decisões por proposta, folds, previews e lock compartilhado da promoção.

## Out of Scope

| Feature | Reason |
| --- | --- |
| Retomada de promoção interrompida e persistência de sessão/plano para esse fim | Usuário: "não quero incluir nada de retomada agora". |
| Detecção e coordenação de promoções simultâneas | Usuário aceitou remover; o time coordena uma promoção por vez. |
| Retry, rollback e recuperação automática de falhas | Usuário aceitou remover; informar o erro e interromper basta nesta versão. |
| Rebase, merge e reconciliação automática quando `main` muda durante a revisão | Usuário aceitou remover; conflitos não serão resolvidos automaticamente. |
| Reabertura de PR fechado e ciclo de abandono/cancelamento com limpeza automática | Usuário aceitou remover; o percurso suportado termina em merge humano e limpeza. |
| Migração de promoção antiga ou dois fluxos completos em paralelo | Usuário aceitou remover; substituir o fluxo antigo. |
| Divisão automática quando o conjunto excede o contexto | Usuário aceitou remover; explicitar a limitação. |
| Publicação especial quando todas as propostas são descartadas e não há alteração canônica | Usuário aceitou remover; não criar caminho alternativo de limpeza direta nesta versão. |
| Arquivo Markdown separado para o plano de promoção | Usuário aprovou discutir e aprovar o plano na conversa. Os artefatos desta especificação documentam o desenvolvimento, não são arquivos de runtime do `/promote`. |
| Scheduler, filas persistentes, locks por página e schema de grupos | A direção da fonte os exclui; delegação é estratégia do skill. |
| Interface própria de revisão no VS Code ou abertura automática do editor | Usuário aprovou apenas informar o caminho e usar arquivos e diff comuns. |
| Reorganização de memórias sem relação com as propostas | Usuário aprovou limitar as mudanças ao plano da consolidação. |
| Redesenho de `/remember`, busca, categorias ou formato canônico | São consumidores e comportamentos existentes que a simplificação deve preservar. |
| Nova política de autenticação, modo offline, rate limiting ou telemetria | Usar GitHub, `gh`, rede e permissões já exigidos pelo projeto. |

## Assumptions & Open Questions

| Assumption / decision | Chosen default | Rationale | Confirmed? |
| --- | --- | --- | --- |
| Base e propostas | Branch a partir de `origin/main`; insumos de uma revisão fixa de `origin/proposals`, fora do commit canônico | Evitar histórico de propostas na `main`; user: "concordo, aprovado" | y |
| Interface | Conversa no terminal e caminho para edição opcional no VS Code | Revisar os mesmos arquivos, sem abertura automática; user: "concordo, aprovado" | y |
| Publicação | PR para `main`, um commit global; limpeza após merge em nova chamada ou comando explícito | User: "concordo, aprovado" | y |
| Escopo inicial | Cenário feliz, sem retomada e sem os sete cenários de exceção discutidos | User: "não quero incluir nada de retomada agora" e "aceito remover todos estes pontos" | y |
| Plano | Discutir e aprovar na conversa, sem arquivo de plano separado | User: "sim", em resposta às três escolhas finais | y |
| Delegação | Principal decide quando delegar memórias independentes após aprovação do plano e revisa o conjunto | User: "sim", em resposta às três escolhas finais | y |
| Reorganização | Pode mudar estrutura das memórias relacionadas, conforme o plano; mudanças sem relação ficam fora | User: "sim", em resposta às três escolhas finais | y |
| Compatibilidade canônica | Preservar categorias e IDs existentes e completar IDs novos antes da revisão final | Requisito da leitura existente, confirmado em `internal/memory/schema.go` e `page.go`; não redesenhar a leitura | y - code |
| Dependências | GitHub, `gh` autenticado, rede, acesso ao repositório e identidade Git disponíveis | Pré-requisitos do fluxo existente em `docs/features.md` e `internal/forge/github/github.go` | y - code |
| Condições do percurso principal | Uma promoção por vez e ambiente preparado, sem trabalho antigo a migrar | Redução explícita de escopo do usuário | y |
| Registro após publicação | Conservar somente o vínculo PR/propostas e os dados necessários à limpeza; sem reconstruir sessão ou plano | Limpeza posterior aprovada; nome, localização e representação são escolhas técnicas do design | y |

**Open questions:** none. Decisões de produto resolvidas; detalhes técnicos ficam para o design dentro deste escopo.

## User Stories

### P1: Preparar e compreender o conjunto

**User Story**: Como responsável pela consolidação, quero começar com memória publicada e propostas atuais para organizar todo o conhecimento pendente.

**Why P1**: O plano precisa representar o conjunto real e não carregar o histórico de propostas para a memória publicada.

**Acceptance Criteria**:

1. WHEN a pessoa invoca `/promote` em um ambiente preparado THEN o sistema SHALL buscar o remoto e criar uma branch em worktree separado a partir da revisão obtida de `origin/main` (`PROM-01`).
2. WHEN a preparação seleciona a revisão de `origin/proposals` THEN o sistema SHALL disponibilizar o conteúdo integral de todas as propostas dessa revisão como o conjunto fixo da execução (`PROM-02`).
3. WHEN o principal prepara o plano global THEN o skill SHALL orientar a leitura integral de todas as memórias publicadas da base selecionada, permitindo busca lexical apenas como auxílio de navegação (`PROM-03`).
4. WHEN uma consolidação contém dezessete ou mais propostas THEN o sistema SHALL permitir seu tratamento no mesmo plano global sem impor lotes de dezesseis (`PROM-04`).
5. WHEN novas propostas chegam após a preparação THEN o sistema SHALL mantê-las fora do conjunto fixo da execução atual (`PROM-05`).

**Independent Test**: Preparar um remoto de teste com dezessete propostas e memórias em mais de uma categoria; conferir base, inventário, bytes dos insumos e exclusão de uma proposta posterior.

### P1: Discutir o plano global

**User Story**: Como pessoa revisora, quero discutir agrupamentos e destinos antes da escrita para aprovar o significado das mudanças.

**Why P1**: A organização do conhecimento e a resolução de contradições pertencem à conversa com a pessoa.

**Acceptance Criteria**:

1. WHEN o levantamento integral termina THEN o skill SHALL apresentar na conversa os agrupamentos com propostas envolvidas, destinos e mudanças sugeridas (`PROM-06`).
2. WHEN o principal apresenta o plano THEN o skill SHALL atribuir a cada proposta inicial um ou mais destinos ou uma justificativa livre para não se tornar memória (`PROM-07`).
3. WHEN o plano relaciona propostas e memórias THEN o sistema SHALL admitir tanto várias propostas para uma memória quanto uma proposta para várias memórias (`PROM-08`).
4. WHILE o plano ainda não foi aprovado o skill SHALL manter os arquivos canônicos sem edições da consolidação (`PROM-09`).
5. WHEN a pessoa solicita unir ou separar grupos, mudar destinos ou resolver interpretações e contradições THEN o skill SHALL incorporar essas decisões ao plano apresentado na conversa (`PROM-10`).
6. WHEN o principal propõe reorganização THEN o skill SHALL limitar a reorganização às memórias relacionadas no plano, preservando conhecimento ainda válido (`PROM-11`).

**Independent Test**: Exercitar o contrato do skill com propostas que se combinam e uma proposta que afeta duas páginas; conferir o plano e a barreira de aprovação antes da escrita. Avaliar preservação semântica na verificação independente.

### P1: Preparar e revisar arquivos finais

**User Story**: Como pessoa revisora, quero receber memórias finais editáveis e seu diff para ajustar o resultado diretamente antes da publicação.

**Why P1**: O arquivo atual deve ser a fonte da revisão e da publicação.

**Acceptance Criteria**:

1. WHEN a pessoa aprova o plano THEN o skill SHALL preparar diretamente os arquivos finais no worktree a partir do conhecimento das propostas, sem copiar automaticamente o documento de proposta com suas instruções de ação e destino (`PROM-12`).
2. WHEN o principal opta por delegar após a aprovação do plano THEN o skill SHALL atribuir cada arquivo a um único responsável por rodada, com conteúdo atual, propostas pertinentes e decisões aprovadas, reservando Git e publicação ao principal (`PROM-13`).
3. WHEN a preparação dos arquivos termina THEN o skill SHALL exigir que o principal revise a coerência do conjunto, incluindo duplicidades, contradições, referências e omissões, antes da revisão humana (`PROM-14`).
4. WHEN memórias são preparadas para a revisão final THEN o sistema SHALL preservar os IDs de seções existentes mantidas e completar os IDs de headings novos conforme o formato canônico atual (`PROM-15`).
5. WHEN os arquivos são validados para publicação THEN o sistema SHALL aceitar somente memórias nas categorias configuradas e com estrutura compatível com a leitura canônica existente (`PROM-16`).
6. WHEN o principal entrega os arquivos para revisão THEN o skill SHALL apresentar um resumo global e o diff por memória usando os arquivos atuais, com o caminho do worktree para edição opcional no VS Code (`PROM-17`).
7. WHILE a pessoa revisa e edita os arquivos o skill SHALL suspender a escrita dos agentes até a pessoa pedir ajustes ou sinalizar pela conversa que terminou (`PROM-18`).
8. WHEN a pessoa pede uma rodada de ajustes THEN o skill SHALL partir dos arquivos atuais e preservar as edições humanas que o pedido não substitui (`PROM-19`).
9. WHEN uma questão de conteúdo muda materialmente o plano aprovado THEN o skill SHALL obter a decisão da pessoa na conversa antes de preparar essa mudança (`PROM-20`).
10. WHEN o principal solicita aprovação para publicar THEN o skill SHALL vincular essa aprovação ao resultado atual mostrado no diff, sem depender de preview token ou versão antiga armazenada (`PROM-21`).

**Independent Test**: Editar arquivos diretamente, incluir proposta livre iniciada com H1 e ação/destino no texto, validar as memórias, simular edição humana e conferir o diff e os bytes publicados. Julgar o contrato de delegação e revisão na verificação do skill.

### P1: Publicar e concluir a consolidação

**User Story**: Como responsável pela publicação, quero enviar as memórias aprovadas para revisão compartilhada e remover apenas as propostas concluídas após o merge.

**Why P1**: A simplificação precisa completar o ciclo sem perder propostas novas nem publicar insumos auxiliares.

**Acceptance Criteria**:

1. WHEN a pessoa aprova a publicação do resultado atual THEN o sistema SHALL criar um commit global da consolidação contendo somente as alterações canônicas aprovadas, excluindo propostas e arquivos auxiliares (`PROM-22`).
2. WHEN a consolidação aprovada é enviada para revisão compartilhada THEN o sistema SHALL publicar a branch, abrir PR com destino à `main` e devolver seu link, deixando o merge para a pessoa (`PROM-23`).
3. WHEN o PR é criado THEN o sistema SHALL registrar no corpo o resumo das propostas concluídas com seus destinos ou justificativas livres de descarte (`PROM-24`).
4. WHILE o merge do PR não foi confirmado pelo GitHub o sistema SHALL manter as propostas concluídas na branch `proposals` (`PROM-25`).
5. WHEN a próxima chamada de `/promote` ou o comando explícito de conclusão confirma o merge THEN o sistema SHALL remover da `proposals` somente os arquivos concluídos cujo caminho e conteúdo correspondam ao conjunto aprovado, preservando propostas novas ou alteradas (`PROM-26`).
6. WHEN a limpeza após merge termina THEN o sistema SHALL atualizar a leitura local para a `main` publicada (`PROM-27`).

**Independent Test**: Criar PR em um GitHub simulado, confirmar retenção antes do merge e limpar depois de squash merge com proposta nova e proposta alterada presentes. Conferir que o diff da `main` contém apenas memórias e que a leitura local passa a refletir a publicação.

### P1: Substituir o contrato antigo

**User Story**: Como mantenedor, quero um único fluxo simples de promoção para que ferramentas e documentação correspondam ao trabalho com arquivos finais.

**Why P1**: Manter a interface antiga transportaria a complexidade removida para o novo fluxo.

**Acceptance Criteria**:

1. WHEN o novo fluxo está disponível THEN o sistema SHALL retirar do contrato de promoção as ferramentas de preview/aplicação individual e em lote, folds, tokens, obrigatoriedade de operação por proposta e transação específica de promoção (`PROM-28`).
2. WHEN uma promoção é preparada ou publicada THEN o sistema SHALL operar sem lock compartilhado de promoção e sem ler ou publicar `release.lock.json` (`PROM-29`).
3. WHEN o fluxo é substituído THEN o sistema SHALL preservar o comportamento de `/remember` e da leitura publicada, incluindo os locks compartilhados que esses componentes ainda usam (`PROM-30`).
4. WHEN o novo fluxo é entregue THEN a documentação SHALL descrever o percurso implementado e retirar instruções de lotes, folds, previews, release lock e avaliação de fontes removida (`PROM-31`).

**Independent Test**: Conferir a lista de ferramentas MCP e comandos expostos, executar o percurso novo e os testes existentes de captura/leitura, e verificar os contratos de documentação e skill.

## Edge Cases

Exceções complexas permanecem fora do escopo. Os seguintes limites evitam declarar sucesso incorretamente, sem mecanismos de recuperação:

1. IF uma operação necessária de rede, Git ou escrita falha THEN o sistema SHALL informar a falha e interromper o percurso sem declarar a consolidação concluída nem apagar o trabalho preparado automaticamente (`PROM-32`).
2. IF o principal não consegue ler integralmente o conjunto no contexto disponível THEN o skill SHALL explicitar a limitação e interromper a preparação do plano, sem apresentar análise parcial como completa (`PROM-33`).
3. IF não existem propostas pendentes na revisão inicial THEN o sistema SHALL informar a ausência de propostas e encerrar sem criar publicação (`PROM-34`).

## Requirement Traceability

| Requirement ID | Story / criterion | Phase | Status |
| --- | --- | --- | --- |
| PROM-01 | Preparar e compreender, AC 1 | Execute | Implementing |
| PROM-02 | Preparar e compreender, AC 2 | Execute | Implementing |
| PROM-03 | Preparar e compreender, AC 3 | Specify | Pending |
| PROM-04 | Preparar e compreender, AC 4 | Execute | Implementing |
| PROM-05 | Preparar e compreender, AC 5 | Execute | Implementing |
| PROM-06 | Discutir o plano, AC 1 | Specify | Pending |
| PROM-07 | Discutir o plano, AC 2 | Specify | Pending |
| PROM-08 | Discutir o plano, AC 3 | Specify | Pending |
| PROM-09 | Discutir o plano, AC 4 | Specify | Pending |
| PROM-10 | Discutir o plano, AC 5 | Specify | Pending |
| PROM-11 | Discutir o plano, AC 6 | Specify | Pending |
| PROM-12 | Preparar e revisar, AC 1 | Specify | Pending |
| PROM-13 | Preparar e revisar, AC 2 | Specify | Pending |
| PROM-14 | Preparar e revisar, AC 3 | Specify | Pending |
| PROM-15 | Preparar e revisar, AC 4 | Execute | Implementing |
| PROM-16 | Preparar e revisar, AC 5 | Execute | Implementing |
| PROM-17 | Preparar e revisar, AC 6 | Execute | Implementing |
| PROM-18 | Preparar e revisar, AC 7 | Specify | Pending |
| PROM-19 | Preparar e revisar, AC 8 | Execute | Implementing |
| PROM-20 | Preparar e revisar, AC 9 | Specify | Pending |
| PROM-21 | Preparar e revisar, AC 10 | Execute | Implementing |
| PROM-22 | Publicar e concluir, AC 1 | Execute | Implementing |
| PROM-23 | Publicar e concluir, AC 2 | Specify | Pending |
| PROM-24 | Publicar e concluir, AC 3 | Specify | Pending |
| PROM-25 | Publicar e concluir, AC 4 | Specify | Pending |
| PROM-26 | Publicar e concluir, AC 5 | Specify | Pending |
| PROM-27 | Publicar e concluir, AC 6 | Specify | Pending |
| PROM-28 | Substituir contrato, AC 1 | Specify | Pending |
| PROM-29 | Substituir contrato, AC 2 | Execute | Implementing |
| PROM-30 | Substituir contrato, AC 3 | Specify | Pending |
| PROM-31 | Substituir contrato, AC 4 | Specify | Pending |
| PROM-32 | Edge Cases, AC 1 | Execute | Implementing |
| PROM-33 | Edge Cases, AC 2 | Specify | Pending |
| PROM-34 | Edge Cases, AC 3 | Execute | Implementing |

**Coverage:** 34 critérios identificados e mapeados às 14 tarefas de `tasks.md`, com 34 checks e provas planejadas.

## Implicit Requirements Review

| Dimension | Resolution |
| --- | --- |
| Input validation & bounds | PROM-02, PROM-04, PROM-15 e PROM-16: leitura integral, sem lote de dezesseis, categorias e formato canônico existentes. |
| Failure / partial-failure states | PROM-32: informar e interromper. N/A para retry, rollback e recuperação, explicitamente excluídos. |
| Idempotency / retry / duplicate handling | N/A para retomada e repetição de publicação: excluídas. Duplicidades de conhecimento fazem parte do julgamento do plano e da revisão em PROM-07 e PROM-14. |
| Auth boundaries & rate limits | N/A para novas políticas: usar identidade Git e acesso GitHub existentes. PROM-21 e PROM-23 mantêm a aprovação e o merge humano. |
| Concurrency / ordering | PROM-18 separa escrita dos agentes de edição humana. N/A para coordenação entre promoções: excluída. |
| Data lifecycle / expiry | PROM-05 e PROM-24 a PROM-27 definem conjunto inicial, vínculo com publicação e limpeza após merge. N/A para TTL ou expiração de sessão: não há retomada. |
| Observability | PROM-17, PROM-23, PROM-32 e PROM-33 mostram diff, link e impedimentos. N/A para telemetria adicional. |
| External-dependency failure | PROM-32. N/A para circuit breakers, fallback offline ou retry automático: fora do escopo. |
| State-transition integrity | PROM-09, PROM-18, PROM-21 e PROM-25 fixam a sequência humana; sem máquina persistente de estados da promoção. |

## Differences from the source

O documento de entrada é um plano de discussão. As respostas posteriores resolvem suas questões abertas e reduzem seu escopo:

| Source | Decision in this specification | Confirmation |
| --- | --- | --- |
| Retomada e recuperação ainda em discussão | Fora desta versão | User: "não quero incluir nada de retomada agora". |
| Concorrência, falhas, volume e compatibilidade como questões abertas | Adiar os sete cenários descritos em Out of Scope | User: "aceito remover todos estes pontos". |
| Base, interface e publicação abertas | `main`, propostas fixas como insumo, terminal/editor e PR com limpeza posterior | User: "concordo, aprovado". |
| Arquivo de plano, critério de delegação e alcance de reorganização abertos | Plano na conversa; principal decide delegação; reorganização ligada às propostas | User: "sim". |

Não há captura de interface nem referência visual fornecida.

## Sources

- `.local/promote-simplification.md`: problema, percurso e simplificações pretendidas.
- `.specs/features/promote-simplification/context.md`: respostas do usuário e exclusões desta discussão, registradas com suas palavras.
- `integrations/claude-code/skills/promote/SKILL.md` e `internal/mcpserver/server.go`: contrato atual a substituir.
- `internal/memory/schema.go`, `page.go` e `internal/config/config.go`: compatibilidade canônica e categorias existentes.
- `internal/release/finish.go`, `internal/forge/github/github.go` e `docs/features.md`: comportamento atual de publicação e dependências existentes.

## Success Criteria

- Uma execução com mais de dezesseis propostas produz um plano global capaz de relacionar propostas e páginas nos dois sentidos.
- Uma proposta livre com H1 e sugestões de ação/destino resulta em memória canônica própria e válida.
- Edição humana feita no worktree aparece no resultado final aprovado e no commit publicado.
- PR contém somente as alterações canônicas e informa o destino das propostas concluídas.
- Depois de squash merge, a limpeza preserva propostas novas e propostas cujo conteúdo mudou.
- O contrato entregue não exige folds, tokens, lock compartilhado ou operações por proposta e não implementa os cenários excluídos.
