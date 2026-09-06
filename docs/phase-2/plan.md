# Plano da Fase 2: fluxo vertical minimo

## Objetivo

Demonstrar de ponta a ponta que conhecimento descoberto em uma sessao pode ser capturado explicitamente, consolidado como estado atual em Markdown, publicado em Git e recuperado em uma sessao posterior, melhorando uma tarefa real sem acesso a sessao original.

O fluxo alvo e:

```text
captura explicita no Claude Code
-> Intake Guard em memoria
-> proposta temporaria de replace
-> diff e revisao humana
-> apply com deteccao de drift
-> commit local path-scoped
-> recuperacao textual em outro processo
```

A fase possui dois gates. O gate tecnico usa um corpus sintetico reproduzivel. O gate de valor usa inferencia autenticada sobre uma descoberta pre-registrada, que pode pertencer a um benchmark sintetico publico, e compara uma tarefa com e sem a memoria recuperada. Passar somente o gate tecnico nao conclui a fase.

## Fontes de decisao

- O [roadmap](../roadmap.md#fase-2-fluxo-vertical-minimo) define o resultado, os entregaveis e o gate de valor.
- A [ADR 0001](../adr/0001-implementation-base.md) fixa Go, a adaptacao Markdown, as invariantes de persistencia, a recuperacao apos interrupcao e o fluxo Git.
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
| operacao | exatamente um `replace_section_content` sobre o corpo de uma secao folha existente; heading, anchor e conjunto de IDs nao mudam |
| alvo | `canonical_relative_path + section_id`; o adapter fornece a identidade e Proposal Workflow resolve e valida o alvo; descoberta por linguagem natural fica fora |
| proposta | JSON v1 sanitizado e imutavel, com `status: pending`; qualquer estado posterior e derivado de receipts |
| estado temporario | root injetavel em testes e privado por usuario; fora do repositorio de memoria |
| revisao | mostrar diff, aplicar como aceite ou rejeitar com reason code fechado; editar uma proposta fica fora da fase |
| aplicacao | `proposal apply <id> <revision>` e o aceite explicito e aplica a revisao; nao existe estado intermediario `accepted` |
| recuperacao | varredura textual deterministica de Markdown, sem indice persistente |
| orcamento | limite em bytes UTF-8 do `ContextBundle` JSON, sem envelope JSON-RPC; estimativa de tokens e apenas metrica |
| publicacao | commit local na branch corrente com indice Git temporario e paths do receipt |
| integracao Claude | tools MCP de proposta, busca e expansao; captura somente por acao explicita |

Limites de admissao sao medidos nos bytes UTF-8 recebidos: candidato JSON ate 256 KiB, `replacement` ate 128 KiB, no maximo 16 `source_paths`, cada path ate 1024 bytes, e `target_path` ate 1024 bytes. O valor exatamente no limite e aceito; um byte ou item acima retorna `limit_exceeded` antes de scan ou persistencia. `section_id` segue o limite de 64 caracteres do schema Markdown.

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
memory_propose_replace
memory_search
memory_expand
```

`memory_propose_replace` recebe path relativo canonico, section ID, corpo substituto completo e referencias de fonte sanitizadas. A tool retorna ID, revisao, alvo e resumo do diff, mas nao oferece apply ou publish. O alvo informado pelo adapter nao e autoritativo: Proposal Workflow resolve a identidade no snapshot do Memory Repository e constroi a operacao. `memory_search` retorna excerpts e handles; `memory_expand` carrega uma secao especifica dentro do orcamento.

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

A preparacao de repositorios Markdown legados sem anchors fica fora da fase. O repositorio usado no cenario real deve ter a pagina alvo preparada e commitada antes do experimento.

### Schema Markdown v1

Uma pagina valida na Fase 2:

- e um arquivo regular `.md`, UTF-8 valido e confinado ao memory root;
- possui pelo menos um heading e exatamente um H1;
- possui um anchor `<!-- @id: slug -->` na linha imediatamente posterior a cada heading;
- possui IDs que casam com `^[a-z0-9]+(?:-[a-z0-9]+)*$`, com no maximo 64 caracteres e unicos dentro do arquivo;
- nao possui conflict markers nem anchors de merge;
- pode possuir frontmatter, mas ele e opcional, nao participa da identidade e permanece byte-preserving.

O alvo deve existir antes de `prepare` e ser uma secao folha. O replacement representa somente seu corpo: nao pode conter heading ATX/setext nem `<!-- @id:`. Validacao de post-state exige o mesmo conjunto ordenado de headings e IDs do baseline. Criacao de pagina, insercao automatica de anchors e migracao de schema nao fazem parte do tracer bullet.

### Politica de privacidade v1

Intake Guard aceita exatamente quatro campos: `target_path`, `section_id`, `replacement` e `source_paths`. Campos desconhecidos, inclusive `session`, `session_id`, `prompt`, `response`, `messages`, `transcript`, `rationale` e `conversation`, sao rejeitados antes de desserializar para AllowedCandidate.

Strings devem ser UTF-8 valido e nao podem conter controles C0/C1, exceto tab, CR e LF em `replacement`. Valores de texto sao examinados exatamente como recebidos. Para `target_path` e `source_paths`, a validacao substitui `\` por `/`, aplica `path.Clean`, rejeita path absoluto ou segmento `..` e examina tanto o valor original quanto o canonico. Nenhuma normalizacao Unicode e aplicada na politica v1.

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
```

T4 pode avancar em paralelo com T5. T7 pode avancar em paralelo com a integracao final de T8 depois que T6 estabilizar o ApplyReceipt. Cada tarefa deve entregar seus testes e documentacao; nao acumular verificacao para T9.

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

## Estrategia de testes

| Nivel | Foco |
|---|---|
| unitario | parser/splice, IDs, ranking, budget, schemas, guard e transicoes puras |
| modulo | cada interface publica contra adapters reais ou fakes privados |
| integracao | filesystem, locks, fsync/rename, Git real e MCP stdio |
| concorrencia | IDs, proposals, locks, apply e `go test -race` |
| fault injection | antes/depois de rename, receipt, cleanup e commit |
| E2E sintetico | fluxo completo e reproduzivel com corpus da Fase 1 |
| experimento real | comparacao cega de valor em exatamente tres pares independentes |

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
- create, append, delete, archive, merge e merge driver;
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
| corpus original nao possui IDs | fixture derivado com anchors; corpus da Fase 1 permanece intacto |
| SDK MCP amplia dependencias | auditoria e lock no mesmo commit da primeira importacao |
| proposta vira segunda fonte de verdade | artefato temporario; Markdown/Git continuam autoritativos |
| status mutavel contradiz proposta imutavel | estado terminal e derivado de receipts separados |
| dado sensivel chega ao disco | Intake Guard antes de store, lock, log ou objeto Git; rescan no apply |
| target escolhido incorretamente pelo modelo | path e section ID obrigatorios; sem descoberta por texto livre |
| target ja contem trabalho do usuario | staged ou unstaged no target bloqueia prepare; outros paths permanecem permitidos |
| escrita perde atualizacao | hash do arquivo completo, lock, rename duravel e recovery testado |
| indice temporario altera trabalho do usuario | reconciliar somente a entrada limpa do target apos commit e preservar todas as demais entradas |
| busca passa apenas pela simplicidade do fixture | ranking/desempate fixos e validacao posterior em cenario real |
| gate de valor vira impressao subjetiva | tres pares isolados, rubrica cega, metrica primaria e limiar pre-registrados |

## Gate de conclusao

A Fase 2 termina somente quando todos os itens forem verdadeiros:

1. Captura explicita chega a proposta, diff, apply, commit e recuperacao.
2. A memoria final representa somente o estado atual esperado.
3. Todas as classes da politica de privacidade v1 sao interrompidas antes da primeira escrita, no corpo e nos metadados.
4. Drift em qualquer parte do arquivo impede apply.
5. Interrupcoes nos pontos definidos sao reconciliadas sem perda ou duplicacao.
6. O commit inclui somente o path aplicado e preserva trabalho alheio.
7. Um processo novo recupera contexto relevante dentro do budget.
8. Prompt de runtime, resposta, transcript e session ID nao sao persistidos pelo produto nem pelos logs do run.
9. Fontes adaptadas e dependencias distribuidas possuem inventario e licencas.
10. No cenario real, o tratamento nao regride em correcao e supera o limiar da metrica primaria em pelo menos dois de tres pares.

Falha nos itens 1 a 9 reabre a implementacao tecnica. Falha no item 10 exige revisar o fluxo ou abandonar a progressao para a Fase 3, mesmo que todos os testes tecnicos passem.
