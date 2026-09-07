# Plano da Fase 2: fluxo vertical minimo

Status do plano: **T1-T16 concluidas; T17-T19 pendentes; Fase 3 bloqueada**.

## Objetivo

Demonstrar de ponta a ponta que conhecimento descoberto em uma sessao pode ser capturado explicitamente, consolidado como estado atual em Markdown, publicado em Git e recuperado em uma sessao posterior, melhorando uma tarefa real sem acesso a sessao original.

O fluxo alvo e:

```text
captura explicita no Claude Code
-> Intake Guard em memoria
-> pesquisa e escolha explicita entre create page, create section ou replace
-> proposta temporaria de uma operacao
-> diff e revisao humana
-> apply com deteccao de drift
-> commit local path-scoped
-> recuperacao textual em outro processo
```

A fase possui dois gates. O gate tecnico usa um corpus sintetico reproduzivel. O gate de valor usa inferencia autenticada sobre uma descoberta pre-registrada, que pode pertencer a um benchmark sintetico publico, e compara uma tarefa com e sem a memoria recuperada. Passar somente o gate tecnico nao conclui a fase.

## Fontes de decisao

- O [roadmap](../roadmap.md#fase-2-fluxo-vertical-minimo) define o resultado, os entregaveis e o gate de valor.
- A [ADR 0001](../adr/0001-implementation-base.md) fixa Go, a adaptacao Markdown, as invariantes de persistencia, a recuperacao apos interrupcao e o fluxo Git.
- A [ADR 0002](../adr/0002-bootstrap-explicito-de-memoria.md) fixa bootstrap sem Markdown manual por operacoes estreitas e mantem o provisionamento Git fora do produto.
- O [mapa de capacidades](../phase-1/capability-map.md#entrada-para-a-fase-2) define as seams e o menor fluxo permitido.
- O [protocolo da Fase 1](../phase-1/evaluation-protocol.md#corpus-comum) fornece o corpus sintetico e as regras de privacidade que devem continuar reproduziveis.
- A [analise de licencas](../phase-1/licenses.md#precondicoes-para-incorporacao) define os criterios de aceite para incorporar fonte de terceiros.

Se este plano divergir da ADR, a ADR prevalece. Uma mudanca na autoridade Markdown/Git, no runtime unico, na identidade de secao ou na fronteira de privacidade exige uma nova decisao arquitetural.

## Decisoes operacionais

| Tema | Decisao da Fase 2 |
|---|---|
| runtime | Go 1.25.x; usar toolchain fixado e build reproduzivel |
| processo | um executavel local; MCP transitorio sobre stdio; nenhum daemon |
| configuracao | `memory_root` e `code_root` explicitos; sem fallback para o diretorio atual |
| operacao | exatamente uma de `create_page`, `create_section` ou `replace_section_content`; propostas multioperacao continuam proibidas |
| alvo | path canonico e, quando aplicavel, section ID; busca sugere contexto, mas a operacao e o alvo sao confirmados explicitamente antes da proposta |
| proposta | JSON v2 sanitizado, imutavel e discriminado por operacao, com `status: pending`; reader continua aceitando JSON v1 persistido por T5-T10 |
| estado temporario | root injetavel em testes e privado por usuario; fora do repositorio de memoria |
| revisao | mostrar diff, aplicar como aceite ou rejeitar com reason code fechado; editar uma proposta fica fora da fase |
| aplicacao | `proposal apply <id> <revision>` e o aceite explicito e aplica a revisao; nao existe estado intermediario `accepted` |
| recuperacao | varredura textual deterministica de Markdown, sem indice persistente |
| orcamento | limite em bytes UTF-8 do `ContextBundle` JSON, sem envelope JSON-RPC; estimativa de tokens e apenas metrica |
| publicacao | commit local na branch corrente com indice Git temporario e paths do receipt |
| integracao Claude | tools MCP de proposta, busca e expansao; `/remember` orquestra pesquisa e confirmacao; captura somente por acao explicita |

Limites de admissao sao medidos nos bytes UTF-8 recebidos: candidato JSON ate 256 KiB, `replacement` ou `body` ate 128 KiB, `title` ou `heading` ate 256 bytes, no maximo 16 `source_paths`, cada path ate 1024 bytes, e `target_path` ate 1024 bytes. O valor exatamente no limite e aceito; um byte ou item acima retorna `limit_exceeded` antes de scan ou persistencia. `section_id` segue o limite de 64 caracteres do schema Markdown.

### Correcao de escopo aprovada apos T10

T8 entregou somente `memory_propose_replace`. O fluxo funciona quando pagina e anchor ja existem, mas exige preparacao manual antes do primeiro uso. Essa lacuna pertence a captura explicita da Fase 2, nao a governanca da Fase 3 nem aos hooks da Fase 4.

Por decisao humana em 2026-09-06, T1-T10 permanecem concluidas e suas evidencias nao sao reescritas. T11-T19 estendem o gate para o produto gerar a primeira pagina e novas secoes e para o fluxo revisado passar por nova validacao de valor.

| Faixa | Estado | Resultado |
|---|---|---|
| T1-T9 | concluida | gate tecnico original de replace reproduzivel |
| T10 | concluida | decisao `revisar o fluxo`; somente 1/3 pares passou o limiar de tempo |
| T11 | concluida | preview canonico de pagina nova, sem mutacao |
| T12 | concluida | proposal/apply/recovery v2 para `create_page`, com reader v1 compativel |
| T13 | concluida | publicacao path-scoped de arquivo novo com baseline ausente |
| T14 | concluida | lifecycle de `create_section` com ID gerado e publicacao existente |
| T15 | concluida | tools MCP e skills explicitos para create page/section |
| T16 | concluida | `/remember` pesquisa, apresenta escolha e confirma uma proposta |
| T17 | pendente | gate tecnico revisado partindo de zero Markdown |
| T18 | pendente | diagnostico e refinamento mensurado de overhead |
| T19 | pendente | novo gate autenticado de valor |

Neste plano, **bootstrap de memoria** significa criar o primeiro arquivo Markdown duravel em um memory root que ja existe como Git worktree local, possui `HEAD` resolvivel, e distinto do code root e nao contem arquivos `*.md`. Um commit inicial vazio e suficiente. Inicializar, clonar ou configurar remotes Git, suportar repositorio unborn e criar paginas em diretorios aninhados nao bloqueiam este gate.

## Interfaces externas minimas

Os nomes abaixo fixam a superficie necessaria para os testes da fase. Flags auxiliares podem ser acrescentadas sem criar caminhos alternativos de escrita.

```text
git-push-memories mcp --memory-root <path> --code-root <path>
git-push-memories proposal show <proposal-id>
git-push-memories proposal reject <proposal-id> --reason <not_relevant|incorrect|sensitive|wrong_target|user_cancelled>
git-push-memories proposal apply <proposal-id> <revision>
git-push-memories proposal publish <proposal-id>
git-push-memories retrieve <query> --budget-bytes <n>
```

Tools MCP:

```text
memory_propose_create_page
memory_propose_create_section
memory_propose_replace
memory_search
memory_expand
```

`memory_propose_create_page` recebe filename canonico na raiz, titulo, corpo inicial e referencias de fonte sanitizadas. O produto deriva page ID e anchor, renderiza o documento e prova que o path esta ausente em HEAD, index e worktree. `memory_propose_create_section` recebe path de pagina existente, heading, corpo e fontes; o produto deriva o novo section ID e propoe uma secao H2 ao final da pagina. `memory_propose_replace` mantem path, section ID, corpo substituto completo e fontes. Todas retornam ID, revisao, alvo e resumo do diff, mas nao oferecem apply ou publish.

O alvo informado pelo adapter nao e autoritativo: Proposal Workflow resolve a identidade ou ausencia no snapshot do Memory Repository e constroi a operacao. `memory_search` retorna excerpts e handles; `memory_expand` carrega uma secao especifica dentro do orcamento. O comando `/remember` e uma instrucao do adapter, nao uma quarta operacao de escrita: ele pesquisa, apresenta a operacao escolhida e exige confirmacao antes de chamar uma das tres tools de proposta.

Os reason codes de rejeicao formam um enum fechado: `not_relevant`, `incorrect`, `sensitive`, `wrong_target` e `user_cancelled`. DecisionReceipt persiste somente um desses valores. Codigo desconhecido falha sem criar receipt; texto livre nao e aceito.

## Contratos dos modulos

As interfaces devem permanecer pequenas e esconder persistencia, validacao e recuperacao. Interfaces Go sao declaradas no pacote consumidor; nao criar uma abstracao para uma dependencia que possui somente uma implementacao.

```text
Intake Guard
inspect(candidate) -> AllowedCandidate | Rejection

Proposal Workflow
prepare(allowedCandidate) -> ProposalView
review(proposalId) -> ProposalView | DecisionReceipt
reject(proposalId, reasonCode) -> DecisionReceipt
apply(proposalId, revision) -> ApplyReceipt | Drift
applied(proposalId) -> ApplyReceipt | NotApplied

Memory Repository
inspect(scope) -> MemorySnapshot
apply(changeSet) -> RepositoryReceipt

Retrieval
retrieve(query, scope, budget) -> ContextBundle
expand(handle, budget) -> ContextBundle

Publisher
publish(applyReceipt) -> PublicationReceipt

Claude Adapter
captureExplicit(candidate) -> ProposalView
rememberExplicit(fact) -> SearchResult + ConfirmedOperation + ProposalView
retrieveContext(query, budget) -> ContextBundle
```

Regras de dependencia:

- somente Memory Repository grava Markdown;
- somente Publisher executa mutacoes Git;
- somente Proposal Workflow conhece o armazenamento de propostas e receipts;
- `proposal show` usa `review`; `proposal publish` obtem um receipt terminal por `applied` antes de chamar Publisher;
- Claude Adapter traduz MCP para os modulos e nao conhece parser, filesystem de propostas ou Git;
- Retrieval nao recebe Proposal Store e permanece estritamente read-only;
- parser, splice, locks, comandos Git e fault injection sao detalhes privados das implementacoes.

## Cenario sintetico

O corpus original da Fase 1 permanece imutavel. O harness cria uma copia derivada de `memory-initial/services/checkout-api.md` com anchors imediatamente depois dos headings. O alvo do tracer bullet e `services/checkout-api.md + checkout-api-operation`.

O estado inicial registra timeout de cinco segundos e ownership do `team-a`. A evidencia em `code-repo/services/checkout-api/service.yaml` registra timeout de tres segundos e ownership do `team-b`. A proposta substitui o corpo da secao `Operacao`; append cronologico nao e aceito como resultado.

T9 e T10 permanecem como evidencias historicas do fluxo de replace e usam pagina preparada. O novo E2E de T17 parte de um commit vazio, cria a pagina sem fixture Markdown e depois exercita criacao e substituicao de secao. Migracao em massa de Markdown legado sem anchors continua fora da fase.

### Schema Markdown v1

Uma pagina valida na Fase 2:

- e um arquivo regular `.md`, UTF-8 valido e confinado ao memory root;
- possui pelo menos um heading e exatamente um H1;
- possui um anchor `<!-- @id: slug -->` na linha imediatamente posterior a cada heading;
- possui IDs que casam com `^[a-z0-9]+(?:-[a-z0-9]+)*$`, com no maximo 64 caracteres e unicos dentro do arquivo;
- nao possui conflict markers nem anchors de merge;
- pode possuir frontmatter, mas ele e opcional, nao participa da identidade e permanece byte-preserving.

Cada operacao acrescenta regras sem alterar o schema Markdown v1:

- `create_page`: `target_path` segue `^[a-z0-9]+(?:-[a-z0-9]+)*\.md$` e aponta para a raiz; o produto deriva page ID do filename, gera H1, anchor e uma secao H2 `Overview` com ID `<page-id>-overview`; `title` e `body` nao podem conter headings nem anchors; ausencia no HEAD, index e worktree e o baseline;
- `create_section`: a pagina alvo deve existir, estar limpa contra HEAD e possuir schema valido; o produto deriva o ID de `<page-id>-<heading-slug>`, acrescenta uma secao H2 ao final e rejeita colisao de ID ou heading; `heading` e `body` nao podem conter headings nem anchors;
- `replace_section_content`: o alvo deve existir e ser uma secao folha; `replacement` representa somente seu corpo e nao pode conter heading ou anchor; o conjunto ordenado de headings e IDs nao muda.

IDs derivados que excedam 64 caracteres ou colidam falham de forma tipada antes de persistencia. O renderer usa LF, uma unica linha vazia entre blocos e newline final. O diff de `create_page` parte de `/dev/null`; clientes nunca enviam frontmatter, anchors ou Markdown estrutural.

### Politica de privacidade v1

Intake Guard aceita um union discriminado fechado. Campos comuns sao `operation`, `target_path` e `source_paths`; `create_page` admite somente `title` e `body`, `create_section` somente `heading` e `body`, e `replace_section_content` somente `section_id` e `replacement`. Campos desconhecidos, inclusive `session`, `session_id`, `prompt`, `response`, `messages`, `transcript`, `rationale` e `conversation`, sao rejeitados antes de desserializar para AllowedCandidate.

Strings devem ser UTF-8 valido e nao podem conter controles C0/C1, exceto tab, CR e LF em `body` ou `replacement`. Valores de texto sao examinados exatamente como recebidos. Para `target_path` e `source_paths`, a validacao substitui `\` por `/`, aplica `path.Clean`, rejeita path absoluto ou segmento `..` e examina tanto o valor original quanto o canonico. Nenhuma normalizacao Unicode e aplicada na politica v1.

Os detectores de conteudo usam RE2:

| Classe | Detector normativo v1 |
|---|---|
| chave privada | `-----BEGIN[ A-Z0-9_-]*PRIVATE KEY-----` |
| authorization | `(?i)\bauthorization\s*:\s*(basic|bearer)\s+\S+` |
| atribuicao sensivel | `(?i)\b(api[_-]?key|access[_-]?token|secret|password)\b\s*[:=]\s*\S+` |
| AWS access key | `\b(AKIA|ASIA)[0-9A-Z]{16}\b` |
| GitHub token | `\bgh[pousr]_[A-Za-z0-9]{36,}\b` |
| Slack token | `\bxox[baprs]-[A-Za-z0-9-]{10,}\b` |
| OpenAI-style key | `\bsk-[A-Za-z0-9_-]{20,}\b` |
| email | ``\b[A-Za-z0-9.!#$%&'*+/=?^_`{\|}~-]+@[A-Za-z0-9](?:[A-Za-z0-9-]{0,61}[A-Za-z0-9])?(?:\.[A-Za-z0-9](?:[A-Za-z0-9-]{0,61}[A-Za-z0-9])?)+\b`` |
| telefone E.164 | `\+[1-9][0-9]{7,14}\b` |
| referencia de sessao | `(?i)\b(session[_-]?id|transcript)\s*[:=]\s*\S+` |
| path de transcript Claude | `(?i)(^|[\\/])\.claude[\\/].*\.jsonl(?:$|[?#])` |

CPF e detectado depois de remover pontuacao quando houver exatamente 11 digitos e os dois check digits forem validos. Sequencias com todos os digitos iguais nao sao CPF valido. Os marcadores privados do corpus entram como casos adicionais somente nos testes; valores literais nao entram em logs ou documentacao.

Qualquer match rejeita o candidato inteiro. Falha ao compilar/inicializar detectores impede iniciar CLI/MCP; erro durante scan rejeita a captura. Erros retornam somente classe e reason code, nunca o trecho. A politica nao tenta classificar semanticamente texto livre como prompt ou resposta: o schema fechado impede esses campos, e a Fase 3 avaliara protecoes semanticas adicionais.

## Ordem de execucao

```text
T1 protocolo e walking skeleton
-> T2 motor Markdown e licencas
-> T3 Memory Repository
   -> T4 Retrieval e MCP de leitura
   -> T5 Intake Guard e proposta
      -> T6 apply, receipts e recovery
         -> T7 Publisher Git
T4 + T5 -> T8 Claude Adapter de captura
T4 + T6 + T7 + T8 -> T9 E2E sintetico
T9 -> T10 experimento real
T10 -> decisao revisar o fluxo
      -> T11 preview de create page
         -> T12 proposta, apply e recovery de create page
            -> T13 publicacao de arquivo novo
         -> T14 create section
T12 + T13 + T14 -> T15 tools e adapter de criacao
T4 + T8 + T15 -> T16 /remember unificado
T13 + T14 + T16 -> T17 E2E de bootstrap
T16 + T17 -> T18 perfil e refinamento operacional
T18 -> T19 novo experimento de valor
```

T4 pode avancar em paralelo com T5. T7 pode avancar em paralelo com a integracao final de T8 depois que T6 estabilizar o ApplyReceipt. Na extensao, T14 pode avancar em paralelo com T12-T13 depois que T11 estabilizar renderer e IDs. Cada tarefa deve entregar seus testes e documentacao; nao acumular verificacao para T17 ou T19.

## T1: protocolo executavel e walking skeleton

**Cenario ou hipotese:** um executavel Go e contratos externos estaveis permitem evoluir os modulos sem acoplar Claude Code, filesystem e Git.

**Escopo:** criar o modulo Go, entrypoint, composicao de dependencias, envelopes JSON versionados, configuracao explicita de roots e transporte MCP stdio. Selecionar e fixar uma versao do SDK MCP oficial compativel com Go 1.25.x.

**Artefatos:** `go.mod`, toolchain fixado, `cmd/git-push-memories`, estrutura inicial sob `internal/`, fixture derivado da Fase 2 e inventario da dependencia MCP.

**Criterios de conclusao:**

- `go test ./...`, `go test -race ./...`, `go vet ./...` e build passam;
- build e testes passam offline depois do download inicial;
- ajuda, versao e inicializacao MCP nao criam estado persistente;
- roots ausentes, iguais quando deveriam ser separados, fora de Git ou invalidos falham sem fallback;
- stdout do modo MCP contem somente protocolo e diagnosticos sanitizados usam stderr;
- versao, licenca, closure e motivo da dependencia MCP estao registrados.

**Dependencia e decisao habilitada:** nao possui dependencia. Confirma a superficie externa e a toolchain usadas pelas tarefas seguintes.

## T2: adaptar o motor Markdown

**Cenario ou hipotese:** os tres arquivos aprovados do agent-memory fornecem edicao por secao sem reformatar regioes nao alteradas.

**Escopo:** adaptar somente `parse.go`, `splice.go` e `section_id.go` da revisao fixada; remover a tolerancia a linha vazia antes do anchor; rejeitar IDs duplicados; manter Goldmark 1.7.4. `validate.go`, `merge.go`, testes e demais fontes upstream nao entram.

**Artefatos:** pacote Markdown privado, golden tests, copias das licencas aplicaveis, inventario de terceiros e marcacao de origem/modificacao nos arquivos adaptados.

**Criterios de conclusao:**

- anchors sao aceitos somente na linha imediatamente posterior ao heading;
- IDs ausentes, duplicados e mal posicionados possuem resultados deterministas;
- headings em code fences nao viram secoes;
- UTF-8, CRLF, ausencia de newline final e secoes adjacentes estao cobertos;
- replace preserva byte a byte tudo fora da secao alvo;
- blobs, revisao, Apache-2.0, Goldmark MIT e modificacoes estao registrados;
- nenhuma fonte fora da lista aprovada aparece no produto ou na distribuicao.

**Dependencia e decisao habilitada:** depende de T1. Libera o motor privado usado pelo Memory Repository.

## T3: implementar Memory Repository

**Cenario ou hipotese:** uma unica porta de escrita consegue esconder schema, paths, parser, hashes, lock e escrita atomica dos demais modulos.

**Escopo:** implementar `inspect` e `apply` para um arquivo Markdown, confinamento ao memory root, validacao de schema e IDs, hash SHA-256 do arquivo completo, leitura do HEAD/blob Git para o snapshot, lock entre processos e escrita por temporario adjacente com `fsync`, rename e `fsync` do diretorio.

**Artefatos:** modulo Memory Repository, testes de integracao em repositorios Git descartaveis e seams privadas para filesystem/fault injection.

**Criterios de conclusao:**

- inspect retorna path canonico, secoes, IDs, hash dos bytes reais, HEAD e blob Git do target;
- replace altera somente o corpo da secao alvo e preserva seu anchor;
- paths absolutos, `..`, symlinks de escape e arquivos fora do root sao rejeitados;
- schema invalido e IDs duplicados impedem qualquer mutacao;
- alteracao concorrente em qualquer byte produz drift;
- dois applies do mesmo baseline resultam em um sucesso e um drift;
- code root, indice Git e objetos Git nao recebem escrita;
- parser e splice nao aparecem na interface dos adapters.

**Dependencia e decisao habilitada:** depende de T2. Libera leitura e escrita canonicas para Retrieval e Proposal Workflow.

## T4: entregar recuperacao textual

**Cenario ou hipotese:** varredura textual por secoes e suficiente para recuperar contexto util no volume inicial sem criar um indice.

**Escopo:** implementar `retrieve` e `expand`, ranking lexical deterministico, desempate estavel, excerpts, handles com path/section/hash e limite em bytes UTF-8. Excluir `.git`, estado temporario, propostas, receipts e arquivos nao Markdown.

**Artefatos:** modulo Retrieval, tools MCP `memory_search` e `memory_expand`, cliente MCP deterministico e testes de read-only/reinicio.

**Criterios de conclusao:**

- a consulta do corpus encontra a secao de operacao;
- depois do replace, somente o estado atual aparece como autoritativo;
- resultados empatados preservam ordem deterministica;
- expand funciona em outro processo e detecta handle obsoleto pelo hash;
- a resposta nunca ultrapassa silenciosamente o budget;
- um budget insuficiente retorna erro tipado em vez de JSON truncado;
- inventarios e hashes comprovam que a recuperacao nao escreve arquivos;
- MCP indisponivel nao bloqueia a sessao Claude Code.

**Dependencia e decisao habilitada:** depende de T3. Prova o primeiro tracer bullet, Claude Code -> MCP -> Markdown, antes da escrita.

## T5: implementar Intake Guard e propostas

**Cenario ou hipotese:** um candidato estruturado pode virar diff revisavel sem persistir conteudo sensivel nem alterar Markdown ou Git.

**Escopo:** implementar Intake Guard em memoria, limites de tamanho, allowlist de campos, verificacao de corpo e metadados, `prepare`, `review`, `reject`, `applied` e Proposal Store privado. A politica inicial rejeita integralmente o candidato; redaction parcial e edicao de proposta ficam fora.

**Artefatos:** tipos Candidate/AllowedCandidate, Proposal Workflow parcial, schema JSON v1, store com diretorio `0700` e arquivos `0600`, diff unificado e receipts de rejeicao.

**Criterios de conclusao:**

- somente Intake Guard consegue criar AllowedCandidate;
- marcadores sensiveis do corpus sao rejeitados no corpo e em qualquer metadado;
- campos de sessao, prompt, resposta, transcript, rationale privada e campos desconhecidos sao rejeitados;
- rejeicao nao cria proposta, lock, log, temporario, receipt de conteudo ou objeto Git;
- erros e logs nao repetem o payload nem persistem seu hash;
- proposta valida contem uma operacao, baseline/post-state, HEAD/blob esperados, diff, evidence refs sanitizadas, revisao, `status: pending`, timestamp e ID unico;
- target com alteracao staged ou unstaged em relacao ao HEAD e rejeitado antes de persistir a proposta;
- proposta recarregada em processo novo produz o mesmo diff;
- prepare e show deixam Markdown, HEAD e indices Git byte a byte inalterados;
- reject aceita somente reason code do enum, grava o code no receipt antes de remover o payload e repeticao retorna o mesmo resultado terminal;
- Proposal Store publica uma proposta somente por temporario, `fsync`, rename e `fsync` do diretorio;
- falha durante prepare nunca torna JSON parcial visivel e temporarios incompletos sao removidos na inicializacao seguinte.

**Dependencia e decisao habilitada:** depende de T3 e pode avancar em paralelo com T4. Libera a fronteira segura de captura.

## T6: implementar apply e recuperacao

**Cenario ou hipotese:** a revisao aceita pode ser aplicada uma unica vez e reconciliada depois de interrupcoes sem perder atualizacao.

**Escopo:** carregar a revisao exata, verificar integridade da proposta, repetir guard/schema/path/Git validation, recalcular hashes/diff, aplicar sob lock, persistir ApplyReceipt e remover o payload somente depois do receipt duravel. Invocar `apply` para a revisao exata constitui o aceite; nao ha transicao separada para `accepted`.

**Artefatos:** `apply`, estados pending/applied_unpublished/rejected, reconciliador de startup, fault injection e testes concorrentes.

**Criterios de conclusao:**

- o caso valido deixa apenas o novo estado na secao;
- drift dentro ou fora da secao rejeita apply sem alterar bytes;
- adulteracao de JSON, operacao, diff ou post-state e detectada;
- conteudo sensivel introduzido depois de prepare e bloqueado no rescan;
- reaplicar uma proposta concluida retorna o mesmo receipt sem nova escrita;
- falha antes do rename preserva baseline e permite retry;
- falha depois do rename e antes do receipt reconhece post-state e reconstrui o receipt;
- se o filesystem apresentar novamente o baseline depois de uma interrupcao nessa janela, o apply permanece elegivel para retry;
- falha depois do receipt e antes do cleanup remove o payload sem reaplicar;
- temporarios controlados e incompletos sao removidos na inicializacao seguinte;
- Proposal/Decision/Apply receipts usam temporario, `fsync`, rename e `fsync` do diretorio antes de qualquer cleanup;
- reject e apply usam o mesmo lock por proposta e compare-and-set de `pending` para um unico estado terminal;
- em corrida reject/apply, somente um receipt terminal existe: se apply vencer, Markdown corresponde ao post-state; se reject vencer, Markdown permanece no baseline;
- estado diferente de baseline e post-state retorna drift sem reparo automatico;
- stress concorrente e `go test -race` nao revelam colisao ou perda de atualizacao.

**Dependencia e decisao habilitada:** depende de T5. Confirma as garantias de durabilidade da ADR e libera Publisher.

## T7: publicar commit Git isolado

**Cenario ou hipotese:** um indice Git temporario permite publicar somente o path aplicado sem consumir trabalho preexistente do usuario.

**Escopo:** implementar Publisher para commit local na branch corrente, validar ApplyReceipt/root/post-state/HEAD, bloquear o indice real pelo protocolo `.git/index.lock`, construir a tree a partir de um indice temporario, avancar a branch por `update-ref` compare-and-swap e instalar um indice real reconciliado por rename atomico. ApplyReceipt registra proposal ID/revisao, memory root, target, hashes, HEAD parent e blob de baseline. O commit recebe trailer `Git-Push-Memories-Proposal: <id>/<revision>`.

O ponto de linearizacao do conteudo e a captura do blob: depois de adquirir o index lock, Publisher confirma HEAD, entrada do target no indice e `post_state_hash` no worktree; em seguida le os bytes, confirma novamente o hash e grava o blob Git. Mudanca observada antes dessa captura aborta. Mudanca de worktree depois da captura nao altera o blob do commit e permanece como unstaged contra o novo HEAD. HEAD e indice continuam protegidos por lock/CAS ate a reconciliacao.

**Artefatos:** adapter Git do Publisher e testes de integracao com staged/unstaged concorrente.

**Criterios de conclusao:**

- a arvore do commit difere do parent somente no path do receipt;
- arquivos alheios staged mantem o mesmo blob no indice real e nao entram no commit;
- arquivos alheios unstaged permanecem inalterados;
- target previamente staged e rejeitado antes da publicacao;
- target staged ou unstaged antes de prepare e rejeitado e nunca entra no baseline da proposta;
- mudanca de HEAD, indice ou post-state detectada antes da captura do blob impede commit sobre estado inesperado;
- depois do commit, a entrada do target no indice real corresponde ao novo HEAD e `git diff --cached -- <target>` fica vazio;
- todas as outras entradas e flags do indice real permanecem byte a byte equivalentes;
- `git add` concorrente e bloqueado pelo index lock; edicao concorrente do target no worktree e preservada como alteracao unstaged contra o novo HEAD;
- testes sincronizados exercitam uma edicao antes da captura, que aborta, e outra depois da captura, que permanece unstaged;
- qualquer divergencia do snapshot esperado aborta antes de `update-ref`, sem sobrescrever indice ou worktree;
- falha Git preserva `applied_unpublished` e permite retry sem reaplicar Markdown;
- crash depois do commit e antes do receipt reconhece parent, trailer, tree e blob do commit existente e nao o duplica;
- recovery so conclui ou remove um index lock quando um marcador duravel prova que ele pertence a operacao; lock de outro processo causa erro sem mutacao;
- nenhum remoto e contatado e nenhum conteudo de sessao entra em mensagem ou receipt.

**Dependencia e decisao habilitada:** depende de T6. Completa a publicacao local segura.

## T8: completar Claude Adapter

**Cenario ou hipotese:** Claude Code consegue criar uma proposta explicitamente e recuperar memoria sem adquirir autoridade de apply ou Git.

**Escopo:** adicionar `memory_propose_replace` ao servidor MCP e uma instrucao/comando explicito de integracao. O adapter aceita somente entrada estruturada e chama Intake Guard e Proposal Workflow; nao infere target a partir de texto livre nem persiste fallback.

**Artefatos:** tool MCP de proposta, material minimo de integracao Claude Code, harness isolado e documentacao de configuracao.

**Criterios de conclusao:**

- uma solicitacao explicitamente iniciada cria exatamente uma proposta;
- a resposta retorna ID, revisao, target e resumo compacto do diff;
- Memory/Git permanecem intactos ate a CLI humana executar apply;
- processo novo consegue mostrar a proposta pela CLI;
- candidato sensivel e rejeitado sem bloquear a sessao nem criar spool/retry;
- ausencia do MCP produz erro visivel e nao persiste o payload;
- tools MCP nao expoem apply, publish, sessao ou transcript;
- nenhum hook de SessionStart, Stop ou PostToolUse e instalado.

**Dependencia e decisao habilitada:** depende de T4 e T5. Completa o lado Claude Code do fluxo explicito.

## T9: executar o E2E sintetico

**Cenario ou hipotese:** todos os modulos compoem o fluxo tecnico completo de forma reproduzivel, offline e sem estado de sessao.

**Escopo:** executar o corpus checkout-api em code root e memory root descartaveis e separados, com HOME/config/state isolados. A consulta classificatoria e `checkout timeout team`; `memory_search` recebe budget de 2048 bytes e `memory_expand`, 4096 bytes. O budget conta somente os bytes UTF-8 do `ContextBundle` JSON canonico, antes do envelope MCP. Preservar somente evidencias sanitizadas em `.artifacts/phase-2/<run-id>`.

**Artefatos:** harness sob testes E2E do produto, evidencia local por run e `docs/phase-2/e2e-synthetic.md` com comandos, resultados e checksums necessarios para reproducao.

**Sequencia classificatoria:**

1. Criar e commitar a memoria inicial derivada com anchors.
2. Recuperar o estado inicial com budget de 2048 bytes.
3. Criar a proposta explicita usando a evidencia sintetica.
4. Provar que proposta e diff nao alteraram Markdown, HEAD ou indices.
5. Mostrar o diff e aplicar explicitamente a revisao exata.
6. Publicar commit local path-scoped.
7. Repetir apply/publish com o mesmo state root para provar idempotencia por receipts.
8. Iniciar processo, HOME, state root e cliente MCP novos, preservando somente o memory root publicado.
9. Recuperar o estado atualizado sem acesso ao estado temporario do primeiro processo.
10. Executar um run separado de privacidade com trace de escrita.

**Criterios de conclusao:**

- pagina final contem tres segundos e `team-b`, sem manter cinco segundos ou `team-a` como estado autoritativo;
- diff semantico fica limitado a secao alvo e anchors preexistentes nao mudam;
- commit contem somente `services/checkout-api.md`;
- staged e unstaged alheios sao preservados;
- recuperacao em processo novo retorna path, section ID e estado novo dentro do budget;
- payload terminal da proposta foi removido e receipts nao contem corpo de sessao;
- marcadores privados aparecem somente no fixture de origem permitido;
- execucao offline registra versoes, comandos, codigos, duracoes, hashes, diffs, inventarios e bytes de contexto;
- `go test ./...`, `go test -race ./...` e `go vet ./...` passam no commit avaliado.

**Dependencia e decisao habilitada:** depende de T4, T6, T7 e T8. Fecha o gate tecnico, mas nao o gate de valor.

## T10: executar experimento real

**Cenario ou hipotese:** uma memoria curta de estado atual reduz trabalho ou evita erro em uma tarefa posterior executada por um agente autenticado.

**Precondicao humana:** escolher e aprovar um repositorio, uma descoberta nao sensivel e uma pagina de memoria com anchors. O repositorio pode ser um benchmark sintetico, publico e versionado no proprio produto; o gate real e a inferencia autenticada, nao o uso de codigo privado ou de producao. Pre-registrar a especificacao sanitizada da tarefa posterior, uma rubrica de correcao, uma metrica primaria e seu limiar antes de executar qualquer sessao.

**Escopo:** realizar uma sessao A de descoberta/captura/publicacao e exatamente tres pares independentes da tarefa posterior. Cada par usa dois worktrees no mesmo commit, HOME/state novos, a mesma especificacao de tarefa, modelo, versao do Claude Code, configuracao, permissoes, ferramentas e limite de contexto. O tratamento habilita somente a memoria publicada; o controle desabilita a integracao de memoria. A ordem dentro de cada par e sorteada por seed registrada. Nenhum resultado de um run e fornecido ao seguinte. Runs extras exigem protocolo novo e nao podem substituir seletivamente nenhum dos tres pares.

**Artefatos:** protocolo pre-registrado, especificacao de tarefa tratada como fixture de avaliacao, seed/ordem, hashes de configuracao, evidencias sanitizadas, comparacao por par e relatorio final com decisao `continuar`, `revisar o fluxo` ou `nao demonstrou valor`. A fixture e escrita antes dos runs e nao e capturada de uma sessao.

**Criterios de conclusao:**

- sessoes de tratamento e controle nao acessam transcript, resultado anterior nem artefato privado da sessao A;
- modelo, configuracao, permissao, code commit, task fixture e budget conferem com os hashes pre-registrados;
- uma rubrica objetiva avalia correcao por testes, fatos esperados ou ambos, sem conhecer qual run recebeu memoria;
- o contexto recuperado no tratamento e relevante, correto e cabe no budget; o controle nao recebe esse contexto;
- tratamento nao regride na rubrica de correcao em nenhum par;
- a metrica primaria melhora alem do limiar pre-registrado em pelo menos dois dos tres pares;
- credenciais e estado do Claude Code ficam fora das evidencias;
- uma resposta autenticada do agente e observada; harness deterministico sozinho nao fecha o gate;
- o relatorio vincula a evidencia a decisao sobre iniciar, ajustar ou abandonar a Fase 3.

**Dependencia e decisao habilitada:** depende de T9. Responde se o fluxo minimo produz valor e encerra ou reabre a Fase 2.

**Resultado registrado em 2026-09-06:** 6/6 runs validos, sem regressao de correcao, mas somente 1/3 pares superou o limiar de reducao de tempo. Decisao `revisar o fluxo`; T11-T19 abaixo sao a resposta aprovada, nao uma alteracao retroativa do protocolo.

## T11: gerar preview canonico de nova pagina

**Cenario ou hipotese:** se o Memory Repository receber intencao estruturada em vez de Markdown arbitrario, ele consegue gerar uma pagina valida e revisavel sem expor schema ou anchors ao cliente.

**Escopo:** adicionar uma operacao read-only de preview para `create_page`. Validar filename raiz, titulo e corpo; derivar page ID e ID da secao `Overview`; renderizar bytes canonicos; calcular post-state hash e diff contra `/dev/null`; inspecionar a ausencia do path em HEAD, index e worktree. O preview nao cria diretorio, arquivo, lock, proposta ou objeto Git.

**Artefatos:** tipos privados da operacao, renderer no pacote Markdown, preview no Memory Repository, erros tipados de path/ID/collision e golden tests.

**Criterios de conclusao:**

- o mesmo input produz exatamente os mesmos bytes, IDs, hash e diff entre processos;
- filename fora da raiz, nao canonico, sem `.md`, reservado ou fora do regex falha antes de scan de conteudo;
- titulo e corpo com heading, anchor, conflict marker, UTF-8 invalido ou ID impossivel falham sem escrita;
- target presente em qualquer um de HEAD, index ou worktree retorna `target_exists`, inclusive arquivo untracked;
- renderer produz exatamente um H1, anchors imediatamente posteriores, uma secao `Overview` e newline final;
- body e normalizado uma unica vez de CRLF para LF na fronteira do renderer e permanece inalterado fora dessa regra;
- preview nao muda inventario, metadata observavel do memory root, HEAD, index ou object database.

**Nao objetivos:** criar repositorio Git, aceitar Markdown estrutural, nested paths, frontmatter customizado ou escolher taxonomia.

**Dependencia e decisao habilitada:** depende de T2 e T3. Estabiliza o post-state que T12 persiste e aplica.

## T12: propor, aplicar e recuperar `create_page`

**Cenario ou hipotese:** ausencia pode ser tratada como baseline explicito, permitindo criar uma pagina uma unica vez e reconciliar crashes com as mesmas garantias do replace.

**Escopo:** introduzir schema JSON v2 discriminado por `operation`; estender Intake Guard, Proposal Workflow, Proposal Store, apply e startup recovery para `create_page`. Representar baseline com `exists: false`, HEAD esperado e prova de ausencia em HEAD/index/worktree, sem hash ou blob ficticio. Manter leitura e operacao de propostas/receipts v1 persistidos.

**Artefatos:** candidate/allowed candidate de criacao, codecs v2, migration tests v1->reader atual, ApplyReceipt capaz de representar criacao, fault injection e testes concorrentes.

**Criterios de conclusao:**

- prepare persiste uma operacao, baseline ausente, post-state completo, diff, fontes sanitizadas, revisao e ID unico;
- Intake Guard examina `title`, `body`, `target_path` e `source_paths` antes de store, lock, log ou Git;
- apply repete guard, path validation e prova de ausencia sob lock antes do rename;
- arquivo criado entre prepare e apply causa drift e nunca e sobrescrito;
- dois applies concorrentes resultam em um sucesso e um drift/idempotent receipt, nunca overwrite;
- crash antes do rename preserva ausencia e permite retry;
- crash depois do rename e antes do receipt reconhece exatamente o post-state e reconstrui o receipt;
- outro conteudo no target durante recovery causa drift sem delete ou reparo automatico;
- reject, show, apply repetido e cleanup preservam as garantias existentes de T5-T6;
- propostas e receipts v1 existentes continuam legiveis sem conversao destrutiva.

**Nao objetivos:** proposal multioperacao, overwrite, upsert, delete ou rollback automatico.

**Dependencia e decisao habilitada:** depende de T11 e reutiliza T5-T6. Libera publicacao segura da primeira pagina.

## T13: publicar arquivo novo com baseline ausente

**Cenario ou hipotese:** o Publisher consegue criar um commit path-scoped de um arquivo novo sem consumir staged/unstaged alheio e sem assumir blob de baseline.

**Escopo:** aceitar ApplyReceipt de `create_page`, validar `baseline.exists: false`, HEAD parent, ausencia da entrada no index do parent e post-state no worktree; gravar blob/tree/commit com indice temporario; reconciliar somente a nova entrada no indice real. Estender recovery de publicacao para reconhecer commit ja criado.

**Artefatos:** suporte de create no Publisher, schema de PublicationReceipt, testes Git reais com target ausente, untracked concorrente, staged/unstaged alheio e crash points.

**Criterios de conclusao:**

- tree do commit difere do parent somente pela adicao do target;
- target presente no parent ou index, ou com bytes no worktree diferentes do post-state do receipt, aborta sem `update-ref`;
- staged/unstaged em outros paths permanece byte a byte preservado;
- edicao do target depois da captura do blob permanece unstaged contra o novo HEAD;
- indice real passa a apontar para o blob publicado e nao perde flags/entradas alheias;
- crash depois do commit e antes do receipt reconhece parent, trailer, tree e blob e nao duplica commit;
- retry de publish retorna o mesmo resultado terminal;
- nenhum remoto e contatado e nenhum blob de baseline inexistente e inventado.

**Nao objetivos:** commit inicial sem parent, push, branch automatica ou publicacao de mais de um path.

**Dependencia e decisao habilitada:** depende de T12 e estende T7. Completa o bootstrap duravel em Git.

## T14: criar nova secao em pagina existente

**Cenario ou hipotese:** quando a busca encontra a pagina correta mas nenhuma secao representa o fato, criar uma secao explicita evita tanto overwrite indevido quanto nova pagina duplicada.

**Escopo:** adicionar preview, proposal, apply e recovery para `create_section`. Derivar heading slug e ID namespaced pelo page ID; inserir uma secao H2 ao final da pagina; usar o arquivo completo como baseline de drift e manter publicacao path-scoped existente.

**Artefatos:** renderer de secao, operacao v2 `create_section`, golden tests, testes de lifecycle e testes de publicacao pelo fluxo T7.

**Criterios de conclusao:**

- pagina inexistente, invalida ou dirty contra HEAD falha antes de proposta;
- heading/ID existente, slug vazio ou ID acima do limite retorna collision tipada;
- preview preserva byte a byte a pagina baseline e acrescenta somente separador, heading, anchor e body canonicos;
- alteracao em qualquer byte entre prepare e apply produz drift;
- crash windows e concorrencia possuem os mesmos resultados deterministas de T6;
- apply e publish repetidos sao idempotentes por receipts;
- retrieval em processo novo encontra a secao pelo ID gerado;
- cliente nao envia anchor, page ID nem section ID.

**Nao objetivos:** inserir em posicao arbitraria, criar subsection H3+, reordenar, mover ou mesclar secoes.

**Dependencia e decisao habilitada:** depende de T11 e reutiliza T5-T7. Libera o caso intermediario entre pagina nova e replace.

## T15: expor criacao no MCP e no Claude Adapter

**Cenario ou hipotese:** tools explicitas permitem ao adapter propor criacao sem receber autoridade de apply/publish e sem aceitar Markdown estrutural.

**Escopo:** adicionar `memory_propose_create_page` e `memory_propose_create_section`, atualizar handshake/capabilities, documentar schemas fechados e fornecer skills explicitas de baixo nivel para diagnostico. As tools chamam Intake Guard e Proposal Workflow e nao escrevem Markdown diretamente.

**Artefatos:** handlers MCP, schemas JSON, testes stdio/harness, instrucoes Claude Code e documentacao de erros.

**Criterios de conclusao:**

- cada chamada valida cria no maximo uma proposta da operacao solicitada;
- resposta contem proposal ID, revisao, operacao, target e resumo compacto do diff;
- campos desconhecidos e conteudo sensivel falham sem spool, retry ou payload em log;
- MCP indisponivel falha de forma visivel e nao cria fallback persistente;
- tools nao expoem apply, publish, raw Markdown, anchors, session ou transcript;
- tools existentes de search, expand e replace permanecem compativeis;
- nenhum hook e instalado.

**Nao objetivos:** escolha automatica da operacao, apply pelo modelo ou API generica de changeset.

**Dependencia e decisao habilitada:** depende de T12 e T14. Disponibiliza os novos primitives para T16.

## T16: entregar `/remember` com escolha explicita

**Cenario ou hipotese:** a pessoa deve informar o fato, nao conhecer path, schema ou anchor; pesquisa e confirmacao explicita conseguem rotear o fato para uma operacao segura.

**Escopo:** criar uma skill/comando `/remember` que recebe o fato, executa `memory_search`, expande no maximo os candidatos necessarios dentro do budget e apresenta uma escolha concreta: substituir uma secao encontrada, criar secao em pagina encontrada ou criar pagina raiz. Antes da tool de proposta, mostrar operacao, target, titulo/heading e fontes e exigir confirmacao humana. Se a busca falhar ou for ambigua, nao persistir fallback.

**Artefatos:** skill Claude Code, decision table, mensagens compactas, contract harness MCP para memory root vazio e populado e guia de uso. A obediencia da skill pelo modelo e observada de forma autenticada em T19, nao simulada como teste deterministico.

**Criterios de conclusao:**

- em memory root sem Markdown, `/remember` oferece `create_page` sem pedir Markdown ou anchor;
- com pagina relacionada e sem secao adequada, oferece `create_section` e permite escolher nova pagina;
- com secao folha relacionada, oferece replace, create section ou cancelamento;
- nenhuma operacao e inferida silenciosamente nem proposta antes da confirmacao;
- a busca nao e chamada de deteccao semantica autoritativa e nao mescla conteudo automaticamente;
- cancelamento, ambiguidade, budget insuficiente ou MCP indisponivel nao criam estado;
- o resultado para a pessoa inclui os comandos CLI exatos de show/apply/publish, que continuam humanos;
- prompts, respostas, transcript e rationale nao entram nos argumentos das tools nem nas evidencias.

**Nao objetivos:** captura automatica, recomendacao sem confirmacao, resolucao de contradicao ou publicacao em uma unica acao.

**Dependencia e decisao habilitada:** depende de T4, T8 e T15. Entrega o fluxo explicito revisado que sera avaliado.

## T17: executar E2E sintetico de bootstrap

**Cenario ou hipotese:** o fluxo completo funciona offline partindo de zero Markdown, sem fixture estrutural preparada por uma pessoa.

**Escopo:** criar code root e memory root Git descartaveis e separados; o memory root recebe somente commit vazio. Pelo MCP/adapter, propor e aplicar uma pagina, publicar, propor e aplicar uma secao, substituir essa secao, publicar cada revisao e recuperar o estado final em processo/HOME/state novos. Executar casos separados de colisao, drift, crash e privacidade.

**Artefatos:** runner E2E dedicado, fixtures somente de fatos/codigo, evidencias sanitizadas e relatorio `docs/phase-2/e2e-bootstrap.md`.

**Criterios de conclusao:**

- inventario inicial comprova zero `*.md` e nenhum anchor fornecido por fixture;
- primeiro diff parte de `/dev/null` e o primeiro commit adiciona somente a pagina gerada;
- create section e replace mantem schema/IDs validos e commits path-scoped;
- processo novo recupera somente o estado atual publicado dentro do budget;
- target concorrente nunca e sobrescrito e recovery cobre os crash points de T12-T13;
- trace de escrita prova roots separados, state privado e ausencia de prompt/transcript/session;
- runner passa offline e preserva somente evidencias permitidas;
- `make verify` e `make verify-offline` passam no commit avaliado.

**Nao objetivos:** medir valor, inferencia autenticada ou substituir a evidencia historica de T9.

**Dependencia e decisao habilitada:** depende de T13, T14 e T16. Fecha o gate tecnico revisado.

## T18: perfilar e refinar o overhead operacional

**Cenario ou hipotese:** o primeiro experimento mostrou correcao sem ganho de tempo consistente; separar startup, retrieval, confirmacao e inferencia evita otimizar por intuicao e identifica friccao controlada pelo produto.

**Escopo:** antes de otimizar, pre-registrar pontos de medicao, a referencia de comparacao derivada de T10 e executar pelo menos 10 repeticoes locais do fluxo sintetico. Medir processo start->MCP ready, search, expand, prepare, show, apply e publish sem registrar payloads. Se o overhead local p50 exceder 25% da duracao de referencia do tratamento ou qualquer etapa local tiver p95 acima de 2 segundos, corrigir a etapa dominante com a menor mudanca e repetir a mesma matriz.

**Artefatos:** protocolo de perfil, runner com clock monotonic, tabela before/after sanitizada e registro de qualquer mudanca de produto.

**Criterios de conclusao:**

- medicao distingue tempo local do produto, espera humana simulada e inferencia autenticada;
- warmup, hardware, revisao, comandos e tamanho do corpus sao registrados;
- artefatos contem somente nomes de etapa, duracoes, codigos e hashes permitidos;
- nenhuma metrica depende de transcript ou conteudo de prompt/resposta;
- regressao funcional, de privacidade ou de budget bloqueia a otimizacao;
- resultado declara se a latencia dominante e controlavel pelo produto e qual mudanca, se alguma, entra em T19.

**Nao objetivos:** prometer ganho de inferencia, criar daemon/cache persistente ou alterar o limiar do experimento depois dos dados.

**Dependencia e decisao habilitada:** depende de T16 e T17. Fornece uma revisao de fluxo mensurada para o novo gate de valor.

## T19: repetir o experimento autenticado

**Cenario ou hipotese:** o fluxo `/remember`, agora capaz de partir de zero Markdown e com overhead conhecido, reduz tempo sem regredir correcao em uma tarefa posterior.

**Precondicao humana:** aprovar novo protocolo antes de qualquer run. O protocolo deve fixar fixture publica/sintetica, modelo, Claude Code, configuracao, budgets, rubrica, metrica primaria, limiar, seed e ordem. Resultados de T10 nao podem ser descartados nem usados como run de T19.

**Escopo:** executar uma nova sessao A que usa `/remember` sobre memory root sem Markdown preparado, seguida de exatamente tres pares independentes tratamento/controle conforme isolamento de T10. O tratamento recebe somente memoria publicada; o controle nao carrega a integracao. Runs extras exigem protocolo novo.

**Artefatos:** protocolo pre-registrado v2, evidencia sanitizada, comparacao por par e relatorio com decisao `continuar`, `revisar o fluxo` ou `nao demonstrou valor`.

**Criterios de conclusao:**

- sessao A cria e publica memoria sem fixture Markdown ou anchor manual;
- 3/3 pares sao validos e comparaveis sob os hashes pre-registrados;
- tratamento nao regride na rubrica de correcao em nenhum par;
- metrica primaria supera o limiar pre-registrado em pelo menos 2/3 pares;
- contexto recuperado e relevante, correto e cabe no budget;
- uma resposta autenticada e observada em cada run, com credenciais fora da evidencia;
- relatorio compara T19 com o diagnostico de T10 sem combinar amostras ou mudar limiares;
- somente decisao `continuar` desbloqueia o inicio da Fase 3.

**Dependencia e decisao habilitada:** depende de T18. Fecha a extensao da Fase 2 e decide a progressao.

## Estrategia de testes

| Nivel | Foco |
|---|---|
| unitario | parser/splice, IDs, ranking, budget, schemas, guard e transicoes puras |
| modulo | cada interface publica contra adapters reais ou fakes privados |
| integracao | filesystem, locks, fsync/rename, Git real e MCP stdio |
| concorrencia | IDs, proposals, locks, apply e `go test -race` |
| fault injection | antes/depois de rename, receipt, cleanup e commit |
| compatibilidade | propostas/receipts JSON v1 persistidos continuam legiveis pelo reader v2 |
| E2E sintetico | fluxo original de replace e fluxo revisado partindo de commit sem Markdown |
| perfil operacional | tempos locais por etapa, sem payload, antes e depois de qualquer otimizacao |
| experimento real | comparacao cega de valor em exatamente tres pares independentes por protocolo |

Testes atravessam as interfaces dos modulos. Se um teste precisar conhecer offsets, arquivos temporarios ou comandos Git internos, usar uma seam privada da implementacao em vez de ampliar a interface publica.

## Evidencias

Cada run classificatorio deve preservar:

- revisao do produto, Go, Git, Claude Code e dependencias relevantes;
- configuracao efetiva com paths normalizados e dados sensiveis removidos;
- comandos, codigos de saida e duracoes;
- hashes e inventarios antes e depois;
- diff Markdown e tree do commit Git;
- estado do indice real antes e depois da publicacao;
- tamanho do contexto retornado e criterio de ranking;
- resultado da busca por marcadores sinteticos;
- resultados dos testes, race detector e fault injection.

Logs brutos nao devem conter prompts de runtime, respostas, transcripts ou payload rejeitado. A especificacao pre-registrada e uma fixture intencional de avaliacao, nao conteudo capturado de sessao. Evidencia rastreada no Git deve ser suficiente para reproduzir o run sem copiar outros dados do cenario real.

## Fora do escopo

- propostas multi-arquivo;
- changeset generico, upsert, overwrite, delete, archive, merge e merge driver;
- inicializacao/clonagem Git, repositorio unborn e configuracao de remote;
- pagina em diretorio aninhado, movimentacao de pagina e taxonomia;
- insercao de secao em posicao arbitraria, subsection, reordenacao ou merge de secoes;
- edicao/revisao de uma proposta existente;
- deteccao semantica de duplicidade ou contradicao;
- claims, quote hashes e relacoes epistemicas;
- validacao ou apply decidido por agente;
- captura automatica e hooks;
- branch automatica, push, pull, pull request e GitHub;
- SQLite, FTS5, cache persistente, embeddings, busca semantica, grafo ou ontologia;
- daemon, worker, backend e armazenamento de sessao;
- migracao geral de Markdown legado sem anchors;
- suporte de producao a Windows/macOS, instaladores ou marketplaces;
- Copilot CLI e OpenCode;
- codigo Palinode ou fonte agent-memory fora dos tres arquivos aprovados.

## Riscos e controles

| Risco | Controle nesta fase |
|---|---|
| corpus original nao possui IDs | T9 preserva fixture historico; T17 gera schema e IDs pelo produto a partir de fatos sem Markdown |
| SDK MCP amplia dependencias | auditoria e lock no mesmo commit da primeira importacao |
| proposta vira segunda fonte de verdade | artefato temporario; Markdown/Git continuam autoritativos |
| status mutavel contradiz proposta imutavel | estado terminal e derivado de receipts separados |
| dado sensivel chega ao disco | Intake Guard antes de store, lock, log ou objeto Git; rescan no apply |
| target ou operacao escolhido incorretamente pelo modelo | busca e sugestao nao sao autoritativas; `/remember` exige confirmacao explicita antes da proposta |
| target ja contem trabalho do usuario | staged ou unstaged no target bloqueia prepare; outros paths permanecem permitidos |
| create sobrescreve arquivo concorrente | ausencia em HEAD, index e worktree faz parte do baseline e e reverificada sob lock |
| IDs gerados colidem ou ficam instaveis | slug/namespace deterministico, validacao de limite e collision error antes de persistencia |
| schema v2 quebra estado temporario existente | reader compativel com JSON v1 e fixtures de migration; sem reescrita destrutiva |
| escrita perde atualizacao | hash do arquivo completo, lock, rename duravel e recovery testado |
| indice temporario altera trabalho do usuario | reconciliar somente a entrada limpa do target apos commit e preservar todas as demais entradas |
| busca passa apenas pela simplicidade do fixture | ranking/desempate fixos e validacao posterior em cenario real |
| fluxo correto continua lento | T18 separa overhead local de inferencia e so otimiza a etapa dominante sob protocolo fixo |
| gate de valor vira impressao subjetiva | tres pares isolados, rubrica cega, metrica primaria e limiar pre-registrados |

## Gate de conclusao

A Fase 2 termina somente quando todos os itens forem verdadeiros:

1. `/remember` parte de um fato e de zero Markdown, pesquisa e exige escolha explicita sem pedir schema ou anchor.
2. `create_page`, `create_section` e `replace_section_content` chegam a proposta, diff, apply, commit e recuperacao.
3. A memoria final representa somente o estado atual esperado.
4. Todas as classes da politica de privacidade v1 sao interrompidas antes da primeira escrita, no corpo e nos metadados.
5. Drift em qualquer parte do arquivo e colisao em target ausente impedem apply.
6. Interrupcoes nos pontos definidos sao reconciliadas sem perda, overwrite ou duplicacao.
7. O commit inclui somente o path aplicado e preserva trabalho alheio.
8. Um processo novo recupera contexto relevante dentro do budget.
9. Prompt de runtime, resposta, transcript e session ID nao sao persistidos pelo produto nem pelos logs do run.
10. Fontes adaptadas e dependencias distribuidas possuem inventario e licencas.
11. O overhead controlado pelo produto foi medido por etapa e qualquer refinamento passou novamente pelo gate tecnico.
12. Em T19, o tratamento nao regride em correcao e supera o limiar pre-registrado em pelo menos dois de tres pares.

Falha nos itens 1 a 11 reabre a implementacao tecnica. Falha no item 12 exige revisar o fluxo ou abandonar a progressao para a Fase 3, mesmo que todos os testes tecnicos passem.
