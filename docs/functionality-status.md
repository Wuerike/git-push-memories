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
| Estado privado | Manter locks operacionais fora de worktrees Git. | **Implementado.** O state root e derivado de XDG e rejeitado quando sobrepoe um worktree Git; o inbox usa somente um lock local. |
| Indice local derivado | Se um indice for necessario, torna-lo descartavel e reconstruivel somente do Markdown. | **Nao aplicavel.** A implementacao atual nao possui indice persistente e percorre os arquivos diretamente. |
| Runtime, banco e store de sessao adicionais | Nao introduzir Python, banco autoritativo, daemon, grafo, vector DB ou store de sessao no primeiro ciclo. | **Implementado como restricao.** Nenhum desses componentes faz parte do produto atual. |

## Markdown e estado atual

| Funcionalidade | Comportamento esperado | Estado atual |
|---|---|---|
| Identidade de secao | Identificar uma secao por path canonico e `section_id` estavel. | **Implementado.** IDs usam `<!-- @id: slug -->` imediatamente depois do heading e sao unicos no arquivo. |
| Parsing byte-preserving | Alterar a secao alvo sem reformatar os demais bytes do documento. | **Implementado.** O parser e o splice adaptados do agent-memory ficam em `internal/markdown`. |
| Validacao de schema | Rejeitar UTF-8 invalido, IDs ausentes ou duplicados, anchors mal posicionados e conflitos de merge. | **Implementado.** O Memory Repository valida o documento antes de ler ou alterar seu estado. |
| Estado atual | Substituir a explicacao autoritativa anterior em vez de acrescentar um delta contraditorio. | **Adiado para curadoria.** A captura apenas registra conhecimento no inbox. |
| Criacao da primeira pagina | Permitir bootstrap sem Markdown ou anchors preparados manualmente. | **Adiado para curadoria.** Uma proposta `create` pode ser registrada sem target. |
| Criacao de secao | Criar uma nova secao quando a pagina existe, mas ainda nao ha alvo adequado. | **Adiado para curadoria.** Uma proposta `update` pode apontar para a pagina relacionada. |
| Operacoes multi-arquivo | Aplicar uma proposta atomica sobre varios arquivos. | **Adiado para curadoria.** |

## Privacidade e captura

| Funcionalidade | Comportamento esperado | Estado atual |
|---|---|---|
| Intake Guard em memoria | Examinar todo o candidato antes de criar proposta, lock, log, spool, cache ou objeto Git. | **Implementado.** O guard opera antes da inicializacao mutavel do inbox. |
| Schema fechado de captura | Rejeitar campos desconhecidos, incluindo sessao, prompt, resposta, transcript e rationale privada. | **Implementado.** A proposta aceita somente titulo, acao sugerida, target opcional, conhecimento e source paths. |
| Deteccao de dados sensiveis | Rejeitar credenciais, PII e referencias de sessao no corpo e nos metadados. | **Implementado para a politica v1.** Ha detectores para chaves, tokens, email, telefone, CPF e referencias de sessao. |
| Privacidade semantica | Reconhecer prompt, resposta ou transcript quando esse conteudo aparece dentro de um campo textual permitido. | **Parcial.** A politica v1 bloqueia campos e padroes conhecidos, mas nao classifica semanticamente texto livre. |
| Fail-closed para captura | Nao persistir uma proposta quando schema, detector ou validacao forem inconclusivos. | **Implementado.** O candidato inteiro e rejeitado. |
| Fail-open para a sessao | Uma falha da memoria deve ser visivel sem bloquear o trabalho no Claude Code. | **Implementado.** Erros das tools sao sanitizados e nao encerram o transporte MCP. |
| Captura explicita | Permitir que a pessoa solicite o registro de um fato sem conceder curadoria ao agente. | **Implementado.** `memory_propose` cria um Markdown no inbox depois de confirmacao. |
| Captura automatica por hooks | Gerar propostas uteis a partir de eventos do Claude Code sem persistir a sessao. | **Adiado para a Fase 4.** Nenhum hook de sessao e instalado atualmente. |

## Propostas e revisao

| Funcionalidade | Comportamento esperado | Estado atual |
|---|---|---|
| Proposta antes da mutacao | Criar um artefato observavel sem alterar Markdown canonico ou consumir staging alheio. | **Implementado.** A proposta e commitada como `proposals/<id>-<slug>.md` diretamente em `main`. |
| Persistencia duravel da proposta | Permitir backup e revisao sem depender do state local. | **Implementado localmente.** A proposta faz parte do historico normal do repositorio e o push permanece humano. |
| Integridade da proposta | Tornar alteracoes na proposta observaveis. | **Implementado pelo Git.** O commit contem exatamente o path retornado. |
| Visualizacao | Permitir que a pessoa inspecione a proposta sem comando proprietario. | **Implementado.** O payload e Markdown legivel no worktree e por `git show`. |
| Rejeicao explicita | Remover uma proposta descartada de forma auditavel. | **Adiado para curadoria.** |
| Aceite explicito | Incorporar conhecimento aprovado na memoria canonica. | **Adiado para curadoria.** Nao existe comando `proposal apply`. |
| Edicao de proposta | Ajustar uma proposta antes da curadoria. | **Disponivel via Markdown e Git comuns.** |
| Consumo do payload | Remover propostas incorporadas junto com a atualizacao canonica. | **Adiado para curadoria.** Ambos deverao ocorrer no mesmo commit. |

## Curadoria e recuperacao de falhas

| Funcionalidade | Comportamento esperado | Estado atual |
|---|---|---|
| Lock de captura | Impedir perda de propostas entre sessoes no mesmo clone. | **Implementado.** Escritores do inbox sao serializados e commitam sobre o `HEAD` mais recente. |
| Concorrencia remota | Resolver propostas criadas em clones diferentes. | **Responsabilidade humana.** Conflitos non-fast-forward aparecem no push normal. |
| Aplicacao, drift e recovery | Transformar propostas em estado canonico com seguranca. | **Adiado para curadoria.** |

## Publicacao Git

| Funcionalidade | Comportamento esperado | Estado atual |
|---|---|---|
| Commit path-scoped | Criar um commit contendo somente a nova proposta. | **Implementado.** `git diff-tree` e coberto nos testes do inbox e MCP. |
| Preservacao do indice | Nao consumir nem perder staging preexistente do usuario. | **Implementado.** O commit usa somente o path da proposta. |
| Branch de captura | Exigir uma base local previsivel para commits concorrentes. | **Implementado.** Captura exige exatamente `main`. |
| Operacoes remotas | Permitir push sem acesso remoto automatico. | **Parcial.** O push e humano; politica de PR e merge continua adiada. |

## Recuperacao

| Funcionalidade | Comportamento esperado | Estado atual |
|---|---|---|
| Busca textual local | Encontrar memoria atual sem banco ou servico externo. | **Implementado.** A busca percorre recursivamente somente categorias declaradas em `gpm.json` e ranqueia secoes lexicalmente. |
| Escopo | Restringir a busca por prefixo canonico de path. | **Implementado.** Paths absolutos e escapes por `..` sao rejeitados. |
| Orcamento de contexto | Nunca ultrapassar silenciosamente o limite de bytes informado. | **Implementado.** Budget insuficiente retorna erro tipado. |
| Handles expandiveis | Retornar path, section ID e hash para expansao posterior e deteccao de conteudo obsoleto. | **Implementado.** `memory_expand` rejeita handles stale. |
| Recuperacao progressiva | Retornar excerpts compactos primeiro e carregar o conteudo integral somente sob demanda. | **Parcial.** Search e expand existem, mas a busca ainda retorna o corpo integral de cada secao selecionada, e nao um excerpt separado. |
| Recuperacao em nova sessao | Encontrar o estado publicado sem acessar proposta, state root ou sessao que originou a descoberta. | **Implementado tecnicamente.** O E2E inicia um novo processo, cliente, HOME, configuracao e state root. |
| Busca semantica e FTS | Melhorar ranking somente depois de falha medida da busca textual. | **Adiado.** Nao ha justificativa atual para introduzir indice, vetores ou grafo. |

## Integracao com Claude Code

| Funcionalidade | Comportamento esperado | Estado atual |
|---|---|---|
| MCP por stdio | Expor captura e recuperacao sem daemon ou porta local. | **Implementado.** O servidor publica `memory_search`, `memory_expand` e `memory_propose`. |
| Setup global | Instalar MCP e `/remember` sem copia manual ou configuracao por projeto. | **Implementado.** `gpm setup <memory-root>` registra o MCP no escopo `user` e instala o skill embutido. |
| Adapter sem autoridade propria | Reutilizar Intake Guard, Inbox e Retrieval sem criar stores ou escrita paralela. | **Implementado.** Os handlers MCP apenas traduzem entrada e saida dos modulos centrais. |
| Restricao de autoridade | Nao expor curadoria ao agente. | **Implementado.** A unica escrita MCP adiciona uma proposta ao inbox. |
| Fluxo `/remember` | Pesquisar, expandir candidatos, apresentar a proposta e pedir confirmacao antes de persistir. | **Implementado no contrato e em harness deterministico.** |
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

As duas matrizes executam testes, race detector, `go vet` e build. Os relatorios
classificatorios da Fase 2 sao evidencia historica do lifecycle supersedido pela
ADR 0004; seus runners nao fazem parte da verificacao atual.
