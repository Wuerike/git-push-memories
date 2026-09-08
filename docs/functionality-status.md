# Estado das funcionalidades

Este documento registra as funcionalidades identificadas durante a Fase 1 e o
estado atual de sua implementacao. A Fase 1 teve como objetivo avaliar projetos
existentes e escolher uma base arquitetural; portanto, sua conclusao nao
significava que todas as capacidades abaixo ja deveriam estar implementadas.

Os estados usados sao:

- **implementado**: comportamento disponivel e coberto pela verificacao atual;
- **parcial**: existe um fluxo funcional, mas parte do comportamento desejado
  ainda nao foi entregue;
- **adiado**: capacidade deliberadamente reservada para uma fase posterior;
- **nao aplicavel**: a condicao somente se aplica caso o componente seja
  introduzido.

## Estado das fases

| Fase | Estado | Evidencia principal |
|---|---|---|
| Fase 1: avaliacao de projetos | concluida | A [ADR 0001](adr/0001-implementation-base.md) escolheu uma implementacao propria em Go com adaptacao seletiva do motor Markdown do agent-memory. |
| Fase 2: gate tecnico | concluido | Os fluxos sinteticos de replace e bootstrap foram executados de ponta a ponta. Consulte [E2E sintetico](phase-2/e2e-synthetic.md) e [E2E de bootstrap](phase-2/e2e-bootstrap.md). |
| Fase 2: gate de valor | aberto | O resultado atual e `revisar o fluxo`; a Fase 3 permanece bloqueada. Consulte o [relatorio do experimento de bootstrap](phase-2/bootstrap-value-experiment-report.md). |

## Arquitetura e armazenamento

| Funcionalidade | Comportamento esperado | Estado atual |
|---|---|---|
| Runtime local | Executar como um unico programa Go, sem daemon, worker ou backend obrigatorio. | **Implementado.** Existe um executavel local e o servidor MCP e transitorio sobre stdio. |
| Markdown e Git canonicos | Manter a memoria legivel e integra sem banco, indice ou processo da ferramenta. | **Implementado.** O Markdown e o historico Git sao a unica autoridade duravel. |
| Configuracao global | Permitir que qualquer sessao use a mesma memoria sem configuracao por repositorio de codigo. | **Implementado.** `setup` registra memory root, MCP e skill uma unica vez no escopo do usuario. |
| Estado privado | Manter propostas fora da memoria e de worktrees Git. | **Implementado.** O state root e derivado de XDG e rejeitado quando sobrepoe um worktree Git. |
| Indice local derivado | Se um indice for necessario, torna-lo descartavel e reconstruivel somente do Markdown. | **Nao aplicavel.** A implementacao atual nao possui indice persistente e percorre os arquivos diretamente. |
| Runtime, banco e store de sessao adicionais | Nao introduzir Python, banco autoritativo, daemon, grafo, vector DB ou store de sessao no primeiro ciclo. | **Implementado como restricao.** Nenhum desses componentes faz parte do produto atual. |

## Markdown e estado atual

| Funcionalidade | Comportamento esperado | Estado atual |
|---|---|---|
| Identidade de secao | Identificar uma secao por path canonico e `section_id` estavel. | **Implementado.** IDs usam `<!-- @id: slug -->` imediatamente depois do heading e sao unicos no arquivo. |
| Parsing byte-preserving | Alterar a secao alvo sem reformatar os demais bytes do documento. | **Implementado.** O parser e o splice adaptados do agent-memory ficam em `internal/markdown`. |
| Validacao de schema | Rejeitar UTF-8 invalido, IDs ausentes ou duplicados, anchors mal posicionados e conflitos de merge. | **Implementado.** O Memory Repository valida o documento antes de ler ou alterar seu estado. |
| Estado atual | Substituir a explicacao autoritativa anterior em vez de acrescentar um delta contraditorio. | **Implementado para secoes folha.** `replace_section_content` troca somente o corpo e preserva heading, ID e formato externo. |
| Criacao da primeira pagina | Permitir bootstrap sem Markdown ou anchors preparados manualmente. | **Implementado.** `create_page` gera deterministicamente H1, secao Overview e IDs. Essa extensao foi aprovada pela [ADR 0002](adr/0002-bootstrap-explicito-de-memoria.md). |
| Criacao de secao | Criar uma nova secao quando a pagina existe, mas ainda nao ha alvo adequado. | **Implementado.** `create_section` gera um H2 e um ID derivado do page ID e do heading. |
| Operacoes multi-arquivo | Aplicar uma proposta atomica sobre varios arquivos. | **Adiado.** Cada proposta atual possui exatamente uma operacao e um arquivo alvo. |

## Privacidade e captura

| Funcionalidade | Comportamento esperado | Estado atual |
|---|---|---|
| Intake Guard em memoria | Examinar todo o candidato antes de criar proposta, lock, log, spool, cache ou objeto Git. | **Implementado.** O guard opera antes da inicializacao mutavel do Proposal Workflow. |
| Schema fechado de captura | Rejeitar campos desconhecidos, incluindo sessao, prompt, resposta, transcript e rationale privada. | **Implementado.** Cada operacao aceita apenas seu conjunto declarado de campos. |
| Deteccao de dados sensiveis | Rejeitar credenciais, PII e referencias de sessao no corpo e nos metadados. | **Implementado para a politica v1.** Ha detectores para chaves, tokens, email, telefone, CPF e referencias de sessao. |
| Privacidade semantica | Reconhecer prompt, resposta ou transcript quando esse conteudo aparece dentro de um campo textual permitido. | **Parcial.** A politica v1 bloqueia campos e padroes conhecidos, mas nao classifica semanticamente texto livre. |
| Fail-closed para captura | Nao persistir uma proposta quando schema, detector ou validacao forem inconclusivos. | **Implementado.** O candidato inteiro e rejeitado. |
| Fail-open para a sessao | Uma falha da memoria deve ser visivel sem bloquear o trabalho no Claude Code. | **Implementado.** Erros das tools sao sanitizados e nao encerram o transporte MCP. |
| Captura explicita | Permitir que a pessoa solicite o registro de um fato sem conceder apply ao agente. | **Implementado.** As tools MCP criam propostas; apply e publish permanecem exclusivos da CLI. |
| Captura automatica por hooks | Gerar propostas uteis a partir de eventos do Claude Code sem persistir a sessao. | **Adiado para a Fase 4.** Nenhum hook de sessao e instalado atualmente. |

## Propostas e revisao

| Funcionalidade | Comportamento esperado | Estado atual |
|---|---|---|
| Proposta antes da mutacao | Criar um artefato temporario e observavel sem alterar Markdown ou Git. | **Implementado.** A proposta e um JSON privado fora dos repositorios. |
| Persistencia privada e atomica | Usar diretorios `0700`, arquivos `0600`, temporario, `fsync`, rename e `fsync` do diretorio. | **Implementado.** O Proposal Store aplica essas garantias a propostas e receipts. |
| Integridade da proposta | Detectar adulteracao de operacao, hashes, diff ou post-state. | **Implementado.** A proposta possui hash de integridade e e revalidada antes do apply. |
| Visualizacao do diff | Permitir que a pessoa inspecione exatamente a alteracao proposta. | **Implementado.** `proposal show` retorna a proposta e seu diff completo. |
| Rejeicao explicita | Rejeitar usando um motivo fechado, persistir o receipt e remover o payload. | **Implementado.** Os motivos aceitos sao `not_relevant`, `incorrect`, `sensitive`, `wrong_target` e `user_cancelled`. |
| Aceite explicito | Aplicar somente quando a pessoa informa a revisao exata. | **Implementado.** `proposal apply <id> --revision <n>` constitui o aceite humano. |
| Edicao de proposta | Ajustar uma proposta criando uma nova revisao imutavel e aplicar exatamente a revisao inspecionada. | **Adiado.** Todas as propostas atuais possuem revisao 1. Para corrigir o conteudo, e necessario rejeitar e criar outra proposta. |
| Remocao do payload terminal | Preservar apenas receipts sem corpo depois de reject ou apply. | **Implementado.** O payload e removido somente depois que o receipt terminal se torna duravel. |

## Apply e recuperacao de falhas

| Funcionalidade | Comportamento esperado | Estado atual |
|---|---|---|
| Drift integral | Revalidar path, schema, privacidade, operacao, HEAD/blob e hashes completos antes de escrever. | **Implementado.** Qualquer divergencia do baseline ou post-state esperado interrompe o apply. |
| Apply atomico | Escrever em temporario adjacente, sincronizar, renomear e sincronizar o diretorio. | **Implementado.** O rename duravel e o ponto de commit do Markdown. |
| Lock e concorrencia | Impedir perda de atualizacao entre processos e produzir somente uma decisao terminal. | **Implementado.** Memory Repository e Proposal Workflow possuem locks e testes de concorrencia. |
| Idempotencia | Repetir apply sem reaplicar a alteracao ou criar um novo resultado. | **Implementado.** O receipt existente e retornado para repeticoes validas. |
| Recuperacao antes do rename | Manter o baseline autoritativo e permitir retry. | **Implementado.** Temporarios controlados incompletos sao removidos na inicializacao seguinte. |
| Recuperacao depois do rename | Reconhecer o post-state e reconstruir o ApplyReceipt ausente. | **Implementado.** O reconciliador diferencia baseline, post-state e drift. |
| Drift irrecuperavel automatico | Nao reparar automaticamente um estado que nao corresponde ao baseline nem ao post-state. | **Implementado.** O workflow retorna drift e preserva os bytes encontrados. |

## Publicacao Git

| Funcionalidade | Comportamento esperado | Estado atual |
|---|---|---|
| Separacao entre apply e publish | Manter Markdown aplicado como `applied_unpublished` ate uma publicacao explicita. | **Implementado.** Falha de Git nao desfaz o Markdown. |
| Commit path-scoped | Criar commit contendo somente o path registrado no ApplyReceipt. | **Implementado.** A tree parte do parent e recebe somente a entrada alvo. |
| Indice Git isolado | Nao consumir nem perder staging preexistente do usuario. | **Implementado.** O commit usa `GIT_INDEX_FILE` temporario e reconcilia o indice real sob lock. |
| Protecao concorrente | Detectar mudancas de HEAD, index ou target durante a publicacao. | **Implementado.** O Publisher combina index lock, hashes e atualizacao compare-and-swap da ref. |
| Idempotencia e recovery de publish | Reconhecer commit ja criado depois de interrupcao e evitar duplicacao. | **Implementado.** Marcadores privados permitem retomar ou reconhecer a publicacao. |
| Operacoes remotas | Criar branch dedicada, push e pull request para revisao compartilhada. | **Adiado para a politica de governanca.** O Publisher atual cria somente commit local na branch corrente e nao acessa remotos. |

## Recuperacao

| Funcionalidade | Comportamento esperado | Estado atual |
|---|---|---|
| Busca textual local | Encontrar memoria atual sem banco ou servico externo. | **Implementado.** A busca percorre arquivos Markdown e ranqueia secoes lexicalmente. |
| Escopo | Restringir a busca por prefixo canonico de path. | **Implementado.** Paths absolutos e escapes por `..` sao rejeitados. |
| Orcamento de contexto | Nunca ultrapassar silenciosamente o limite de bytes informado. | **Implementado.** Budget insuficiente retorna erro tipado. |
| Handles expandiveis | Retornar path, section ID e hash para expansao posterior e deteccao de conteudo obsoleto. | **Implementado.** `memory_expand` rejeita handles stale. |
| Recuperacao progressiva | Retornar excerpts compactos primeiro e carregar o conteudo integral somente sob demanda. | **Parcial.** Search e expand existem, mas a busca ainda retorna o corpo integral de cada secao selecionada, e nao um excerpt separado. |
| Recuperacao em nova sessao | Encontrar o estado publicado sem acessar proposta, state root ou sessao que originou a descoberta. | **Implementado tecnicamente.** O E2E inicia um novo processo, cliente, HOME, configuracao e state root. |
| Busca semantica e FTS | Melhorar ranking somente depois de falha medida da busca textual. | **Adiado.** Nao ha justificativa atual para introduzir indice, vetores ou grafo. |

## Integracao com Claude Code

| Funcionalidade | Comportamento esperado | Estado atual |
|---|---|---|
| MCP por stdio | Expor captura e recuperacao sem daemon ou porta local. | **Implementado.** O servidor publica `memory_search`, `memory_expand` e tres tools de proposta. |
| Setup global | Instalar MCP e `/remember` sem copia manual ou configuracao por projeto. | **Implementado.** `git-push-memories setup <memory-root>` registra o MCP no escopo `user` e instala o skill embutido. |
| Adapter sem autoridade propria | Reutilizar Intake Guard, Proposal Workflow e Retrieval sem criar stores ou escrita paralela. | **Implementado.** Os handlers MCP apenas traduzem entrada e saida dos modulos centrais. |
| Restricao de autoridade | Nao expor apply ou publish ao agente. | **Implementado.** Essas operacoes existem somente na CLI. |
| Fluxo `/remember` | Pesquisar, expandir candidatos, apresentar uma operacao e pedir confirmacao antes de propor. | **Parcial.** O skill e os harnesses deterministas existem, mas o experimento autenticado T19 abortou antes da proposta. |
| Fallback seguro | Falhar de forma visivel quando MCP estiver indisponivel, sem spool ou persistencia alternativa. | **Implementado no contrato e nos harnesses.** |

## Evidencia, consolidacao e governanca futura

| Funcionalidade | Comportamento esperado | Estado atual |
|---|---|---|
| Referencias de fonte | Associar paths de codigo que sustentam a proposta. | **Implementado parcialmente.** Propostas armazenam `source_paths` sanitizados. |
| Proveniencia rica | Associar quote hashes, claims e relacoes `backed_by` e `contradicts` ao fato atual. | **Adiado.** Esses conceitos do Palinode ainda nao possuem contrato implementado. |
| Deteccao de duplicidade | Identificar memoria equivalente antes de criar ou publicar uma nova afirmacao. | **Adiado para a Fase 3.** A busca auxilia a escolha humana, mas nao decide equivalencia. |
| Deteccao de contradicao | Interromper ou encaminhar para revisao propostas incompatíveis com o estado atual. | **Adiado para a Fase 3.** Atualmente a consolidacao depende da operacao escolhida e da revisao humana. |
| Revisao por pull request | Compartilhar uma proposta em branch e PR com historico auditavel. | **Adiado para a Fase 3.** |
| Validacao por agentes | Executar validacao automatizada apenas em pontos sustentados por casos de uso e evidencia. | **Adiado.** Nenhum agente controla apply ou publicacao atualmente. |

## Verificacao

A matriz padrao do repositorio e executada por:

```bash
make verify
```

Depois que a toolchain e as dependencias estiverem disponiveis localmente, a
mesma verificacao pode ser repetida sem acesso ao proxy de modulos:

```bash
make verify-offline
```

As duas matrizes executam testes, race detector, `go vet` e build. Os E2Es
classificatorios e experimentos autenticados possuem protocolos proprios e nao
sao repetidos automaticamente pela suite comum.
