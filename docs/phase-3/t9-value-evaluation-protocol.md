# T9: protocolo pre-registrado de avaliacao de valor

> Registro historico. T9 e T10 foram retiradas do plano por decisao do usuario em 2026-09-11. O [plano revisado](plan.md#revisao-de-escopo-em-2026-09-11) encerra a Fase 3 em T1-T8 e prioriza o uso da ferramenta. As regras abaixo descrevem o experimento anterior, sem tarefa de execucao pendente.

Status: pre-registrado em 2026-09-11; nenhuma sessao autenticada ou run classificatoria foi executada.

## Ponto fixo e hipotese

O produto avaliado e o commit `e92a96b6bc4c6f2f169b8b8e96a419c9d9a1b9b2` (`test: add phase 3 technical e2e`). O corpus, a rubrica, as atribuicoes e as regras executaveis ficam congelados por `testdata/phase-3/value-experiment/protocol.json`. A futura implementacao do runner pode mudar somente mecanica que nao altere esses criterios; qualquer mudanca de hipotese, corpus, arm, metrica, limiar, regra de validade ou decisao exige outro experiment ID e outro commit antes de observar outcomes.

A hipotese e que a memoria canonica depois da curadoria atual melhora uma tarefa posterior de engenharia em relacao ao mesmo agente sem a memoria relevante, sem regressao de correcao, privacidade ou custo operacional inaceitavel.

Os protocolos e resultados da Fase 2 sao registros historicos de outro lifecycle. Nenhuma sessao, proposta, dominio ou conclusao desses experimentos participa desta avaliacao. Os seis dominios sinteticos tambem excluem `parcel-dispatch` e `checkout-api`.

## Corpus congelado

O corpus `phase3-held-out-policies-v1` usa apenas fixtures sinteticos versionados em `testdata/phase-3/value-experiment/`:

- duas fontes commitadas, uma vigente e uma explicitamente aposentada;
- um modulo Go com testes publicos verdes e seis politicas incompletas;
- seis tarefas held-out: lease renewal, webhook grace, settlement, routing regional, feature rollout e cache TTL;
- um evaluator externo com um teste inicialmente vermelho por tarefa;
- um memory root inicial com memoria correta, ausente, desatualizada e uma secao aposentada;
- doze casos de curadoria: nove canonicos e tres negativos;
- dois controles de captura: dedupe byte-identico e rejeicao de segredo normativo.

As nove decisoes canonicas exercitam duas vezes `create_page`, duas vezes `create_section`, tres vezes `replace_section`, uma vez `remove_section` e uma vez `remove_page`. As tres negativas exercitam `discard_proposal` para duplicidade semantica entre propostas, afirmacao incorreta e conteudo sensivel. Seis propostas canonicas devem ser aceitas sem correcao dos campos propostos; as demais exercitam correcao de conteudo, retargeting ou resolucao de conflito. O corpus fixa operacao, target, heading ou section ID quando aplicavel, literais obrigatorios e proibidos no post-state, reference integrity e semantic support como criterios separados.

Os hashes SHA-256 de todos os fixtures, dos skills `/remember` e `/curate` e do evaluator estao no protocolo machine-readable. `go test ./tests/experiment` recalcula esses hashes, valida as classes e prova que cada teste publico parte verde e cada teste oculto alvo parte vermelho.

## Ambiente e isolamento

A execucao T10 deve ocorrer somente depois de revisao deste protocolo, commit em worktree limpo e aprovacao humana explicita para iniciar sessoes autenticadas. O ledger sanitizado deve entrar em `started` antes do primeiro preflight autenticado. O preflight registra o model ID exato retornado pelo alias `sonnet`; todas as sessoes classificatorias devem usar esse ID, Claude Code `2.1.267`, effort `medium`, autocompact em `100k` e os limites congelados. O ambiente usa Linux amd64, Go `1.25.12`, Git `2.54.0`, bubblewrap `0.6.1`, socat `1.7.4.1`, locale `C.UTF-8`, timezone UTC e ambiente herdado vazio. Divergencia de versao exige outro protocolo.

Cada tentativa autenticada usa processo, `HOME`, configuracao XDG/Claude, state root, auth temporaria e diretorio temporario novos, sem session persistence. Credencial temporaria usa modo `0600`, nunca entra nos repositorios ou evidencia e e removida ao terminar o processo. Nao sao carregados hooks, settings, memoria ou skills do usuario; somente os dois skills de producao com hash congelado podem participar de captura e curadoria.

Claude e `gpm` rodam em namespaces bubblewrap irmaos ligados somente por Unix socket e `socat`. Para captura e curadoria, ambos montam a mesma source repository read-only em `/workspace`, de modo que o root passado para snapshot resolve igual; apenas o namespace `gpm` monta memory root em `/memory` e state root em `/state`. Para tarefas, somente Claude monta o task repository em `/workspace` e as tools de referencia sao negadas. Repositorio deste protocolo, gold labels, assignments, evaluator, outros worktrees, memory root, state root e argumentos do servidor nunca sao visiveis a Claude. Antes de cada prompt, uma probe sem inferencia deve falhar ao acessar canary paths ocultos e listar exatamente a arvore permitida; tentativa de tool fora de `/workspace` ou canary oculto no stream invalida a sessao.

As fontes formam um repositorio Git deterministico sem remote: os dois fixtures sao copiados para `deploy/current-policy.yaml` e `docs/retired-policy.md`, com SHA-1, branch `main`, identidade `GPM Fixture <fixture@example.invalid>`, timestamps `2000-01-01T00:00:00Z` e mensagem `fixture: source evidence`. O commit esperado e `0a96d0cab8a4284d09e47c681cc2fd7d63c07ba5`, tree `405cbca380f46db1b59539c8885f4e6962bd343d` e repository identity `git-lineage-sha256:6403e88c87b51be58292c139e2865d746badca3d24764752aa9279d832c622da`. O teste recompoe e valida esses IDs.

Cada sessao de tarefa recebe clone fisico novo do code seed, sem hardlinks e sem remote. `evidence/current-policy.yaml` aparece como `deploy/current-policy.yaml`; `evidence/retired-policy.md` aparece como `docs/retired-policy.md`. Task e evaluator ficam fora do worktree visivel ao agente. O evaluator e injetado somente depois da sessao e recebe um run ID opaco, nunca o arm.

Os dois arms recebem a mesma task, source repository, `gpm.json`, memoria canonica nao relacionada, propostas vazias, inventario MCP, tools, modelo, prompt, budgets e isolamento. A unica diferenca e memoria canonica relevante ao cenario: tratamento recebe os bytes reais do post-state curado; controle aplica exatamente o `control_omission` do cenario no corpus. Webhook e routing omitem somente a secao indicada; lease, settlement, feature e cache omitem a pagina indicada. O controle continua com `memory_search` e `memory_expand` disponiveis. Busca de memoria nao e obrigatoria e a ausencia de chamada e outcome valido.

## Captura e curadoria

As quatorze tentativas de captura incluem treze sessoes autenticadas independentes: uma por caso de curadoria e uma tentativa com segredo sintetico normativo. O decimo quarto controle nao usa modelo: imediatamente depois de `cur-01`, um cliente MCP novo repete uma vez os argumentos exatos de `memory_propose` mantidos apenas em memoria. A repeticao deve retornar `duplicate=true` sem commit novo. O segredo e `ghp_` seguido pelos primeiros 36 caracteres hexadecimais de SHA-256 de `gpm-phase3-value-v1:normative-secret`, sem newline; somente seu hash fica no protocolo. O revisor confirma os campos exatos para forcar uma unica chamada `memory_propose`. O controle passa somente se Intake Guard rejeitar essa chamada antes de criar state file, proposta, commit ou qualquer objeto Git, inclusive loose ou inalcancavel; recusa anterior do agente e outcome valido que falha o controle.

Cada sessao comum recebe o template congelado e uma confirmacao humana literal depois que os campos fechados sao exibidos. `cur-10` e capturada enquanto apenas `cur-01` existe como proposta, portanto nao duplica memoria canonica durante `/remember`; a ordem de curadoria consome `cur-01` antes de avaliar `cur-10` como duplicidade semantica. O revisor confirma a captura sintetica mesmo quando sua semantic support e `contradicts` ou `unclear`, mas nunca confirma campo extra, referencia inventada, fallback ou segundo propose. `cur-12` omite referencia somente com concordancia explicita.

As doze propostas seguem a ordem congelada por SHA-256 de `gpm-phase3-value-v1:curation:<case-id>`. Cada uma recebe uma unica sessao com o template `/curate`. O processo nao pode ler corpus, rubrica, tasks, evaluator, protocolo, gold labels ou outputs anteriores. Um unico revisor humano recebe apenas proposta, fonte vigente e aposentada, memoria atual e preview estruturado. Ele registra sua decisao antes de confirmar; cancelamento, classificacao alterada ou `keep_pending` encerram esse caso como outcomes, sem segunda invocacao, processo substituto ou retry.

Cada confirmacao aplica exatamente um caso. Um caso so e correto quando todos os gold fields do corpus, literais de post-state, paths alterados, remocao da proposta no mesmo commit, reference integrity e semantic support da proposta coincidem. Em create ou replace marcado `without_proposal_correction`, o conhecimento deve aparecer byte-identico; em remove, a classe exige apenas action/target originais e as ausencias registradas. O runner registra apenas classe, disposicao, operacao, target, classe de correcao, status de referencia, semantic support, tempos, turnos, custos quando disponiveis, contagens de preview/confirmacao e metadados Git sanitizados. Conteudo bruto nao e preservado.

## Pares e ordem

Existem dois pares por cenario e doze pares no total. Cada par executa uma sessao de controle e uma de tratamento em processos e clones novos. No primeiro par de cada cenario, a ordem vem do primeiro byte de SHA-256 de `gpm-phase3-value-v1:<numero-do-par>`: impar coloca tratamento primeiro; par coloca controle primeiro. O segundo par usa a ordem oposta. O resultado congelado tem seis pares iniciados por cada arm e um de cada ordem por cenario; hashes e ordens estao materializados em `protocol.json`.

Nao existe retry seletivo. Uma run invalida permanece no ledger e nao e substituida. Repetir o experimento depois de observar qualquer outcome exige novo ID, seed, protocolo e commit.

## Avaliacao cega

O evaluator executa primeiro sem receber o mapa de arms. Antes de injetar o evaluator, executa todos os testes publicos, registra tree e metadados do diff, compara por AST as seis assinaturas exportadas e exige que todo path alterado corresponda a `policy/*.go`. Depois copia o unico arquivo oculto e executa apenas o teste pre-registrado do cenario com `-run`. A rubrica vale dez pontos:

| Check | Pontos |
|---|---:|
| teste oculto do cenario | 4 |
| testes publicos | 2 |
| API exportada preservada | 2 |
| diff limitado a Go sob `policy/` | 2 |

O correctness gate exige teste oculto, testes publicos e API preservada. O score ainda registra scope separadamente. Somente depois de congelar `blind-evaluations.json` o runner junta assignments e calcula os pares.

Exploracao de codigo conta cada invocation event de `Read`, `Grep` e `Glob`, inclusive erro, vazio e retry. Bytes sao o tamanho UTF-8 do result content serializado entregue ao modelo antes do framing. Chamadas de busca de memoria, edicao e teste nao contam como exploracao. Uma recuperacao relevante exige `memory_search` ou `memory_expand` no tratamento e substring ASCII case-insensitive de ao menos um `relevant_terms` no conteudo canonico retornado; o runner guarda somente booleano, contagem e bytes, nunca o resultado bruto. O p90 de contexto de curadoria usa a soma dos resultados MCP por caso.

Um par e `utility_success` quando:

1. o tratamento passa o correctness gate;
2. o score do tratamento nao e inferior ao controle;
3. o tratamento supera o score do controle, ou empata com controle que explorou codigo e reduz em pelo menos 25% tanto chamadas quanto bytes de exploracao.

Duracoes sao wall-clock monotonic do envio do prompt ate o processo terminar, incluindo espera pela revisao humana. Medianas e p90 usam nearest-rank. A razao de tempo divide a mediana das doze sessoes de tratamento pela mediana das doze de controle; a razao de custo divide as somas reportadas. Custo deve existir em todas as sessoes classificatorias ou em nenhuma; disponibilidade mista invalida a avaliacao. Sem custo marginal reportado, os tetos impostos de USD 0,10 no preflight, USD 0,30 por captura, USD 0,50 por curadoria e USD 0,50 por tarefa somam no maximo USD 22,00 e, com duracao, formam o gate; turnos sao somente diagnostico.

## Limiares para avancar

Todos os itens abaixo devem passar:

| Area | Limiar |
|---|---|
| privacidade | zero incidente e zero evidencia privada persistida |
| confianca canonica | 9/9 casos canonicos corretos; nenhuma afirmacao incorreta ou sem suporte |
| decisoes negativas | 3/3 descartes com motivo correto |
| preservacao | ao menos 6/6 aceites esperados sem correcao da proposta |
| referencias | zero representacao incorreta de reference integrity; semantic support registrado separadamente |
| curadoria | mediana por caso <= 180 s; p90 <= 360 s; p90 de resultados de tools <= 64 KiB |
| fluxo completo | mediana captura + curadoria <= 300 s por caso |
| captura | 14/14 tentativas concluidas, dedupe exato passa e segredo normativo e rejeitado |
| completude | 12/12 curadorias, 12/12 pares e 24/24 sessoes de tarefa validas |
| correcao | tratamento correto em 12/12; zero regressao de correctness ou score contra controle |
| recuperacao | memoria relevante recuperada em ao menos 10/12 tratamentos; <= 8 KiB de resultados de memoria por tarefa |
| utilidade | ao menos 9/12 pares com `utility_success` e ao menos 1/2 em cada cenario |
| exploracao | somas de chamadas e bytes no tratamento ao menos 25% menores que no controle |
| tempo | razao tratamento/controle da duracao mediana <= 1,25 |
| custo reportado | razao tratamento/controle do custo total <= 1,25, quando reportado |

Os limites absolutos sao USD 0,10 no preflight, USD 0,30 por captura, USD 0,50 por curadoria, USD 0,50 por tarefa e USD 25 para toda a execucao. Uma sessao que atingir limite ou timeout e invalida como infraestrutura, sem reposicao.

## Validade

Invalidam uma sessao: arm ou mounts errados; probe do sandbox falha; vazamento de assignment, gold label ou evaluator; estado de sessao anterior montado; model ID errado ou misturado; resposta nao autenticada; timeout ou crash de infraestrutura; parser quebrado; MCP indisponivel depois do preflight; tools diferentes da allowlist; ou evaluator inexequivel. Persistencia privada e classificada separadamente como incidente de seguranca com precedencia de `interromper`.

Sao outcomes negativos validos: nao chamar memoria; codigo ruim; teste publico ou oculto falhar; scope falhar; interacao `/remember` ou `/curate` ruim; revisor cancelar evidencia ambigua; proposta permanecer pendente; ou erro normal de dominio. Esses outcomes permanecem no denominador intent-to-treat.

Qualquer sessao invalida impede `avancar`, permanece visivel e produz `revisar_o_fluxo`, salvo se uma regra de seguranca com precedencia exigir `interromper`.

## Regra de decisao

A primeira regra aplicavel decide:

1. `interromper`: qualquer incidente de privacidade, evidencia privada bruta, aceite do segredo normativo, afirmacao canonica sensivel/incorreta/sem suporte, representacao incorreta de reference integrity ou regressao de correctness/score no tratamento.
2. `revisar_o_fluxo`: qualquer sessao invalida ou menos de todas as sessoes requeridas concluidas.
3. `interromper`: com todas as sessoes validas, no maximo 5/9 casos canonicos corretos ou no maximo 3/12 utility successes.
4. `avancar`: todos os limiares de seguranca, curadoria, recuperacao, utilidade, tempo, custo, contexto e completude passam.
5. `revisar_o_fluxo`: seguranca e nao regressao passam, mas algum limiar de `avancar` falha.

`tests/experiment/protocol.go` implementa esta precedencia, o calculo de `utility_success` e nearest-rank; seus testes cobrem os limites exatos. Somente `avancar` em T10 autoriza iniciar a Fase 4.

## Evidencia sanitizada

O runner T10 deve criar `.artifacts/phase-3/<run-id>/` novo e ignorado pelo Git. A allowlist de nomes e a denylist de campos estao em `protocol.json`. Sao permitidos apenas protocolo copiado, preregistration, verificacao, ledger, resultados normalizados, avaliacoes cegas, manifest Git, resumo de privacidade, relatorio e checksums.

Nao podem ser persistidos prompt ou resposta de sessao, transcript, session state, reasoning, credencial, source excerpt, source root, preview token, transaction marker, diff bruto, payload MCP, stderr ou conteudo de tool result. Templates sinteticos estaticos no protocolo nao sao outputs de sessao. Streams autenticados sao consumidos em memoria; somente IDs, booleanos, enums, numeros, paths relativos permitidos e hashes entram na evidencia. A denylist de chaves e aplicada recursivamente sem distinguir maiusculas.

Antes do cleanup, o runner procura em evidencia, memory root, todo objeto Git alcancavel ou inalcancavel enumerado por `git cat-file --batch-all-objects --batch-check`, state root, task worktrees, config privada e tmp: segredo normativo derivado em memoria, canary exclusivo da fonte, path absoluto da fonte, preview tokens, marker, bytes de auth e matches do secret scanner fora de sua origem efemera permitida. O inventario de object IDs e arquivos do object store antes e depois do controle de segredo deve ser identico. Qualquer match proibido ou objeto novo e incidente de privacidade e decide `interromper`. O relatorio T10 deve ligar cada metrica ao limiar e registrar explicitamente runs validas e invalidas antes de aplicar a tabela de decisao.

## Execucao futura

T9 nao fornece autorizacao implicita para gastar budget ou iniciar Claude. T10 deve primeiro implementar e verificar o runner contra este protocolo; depois deve mostrar comando, limite maximo e evidence root e obter confirmacao humana explicita. Nenhuma etapa executa fetch, pull, push, clone remoto ou publicacao externa.
