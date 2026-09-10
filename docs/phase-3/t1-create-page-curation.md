# T1: curadoria de proposta create para pagina canonica

Status: **implementada e verificada em 2026-09-10**.

## Hipotese

Uma proposta explicita de criacao pode se tornar memoria canonica recuperavel por uma skill curta, revisavel e segura, sem edicao manual de Markdown e sem reintroduzir o lifecycle privado removido pela ADR 0004.

## Ponto fixo e seam

- Ponto fixo da implementacao: `17e0561269b1c8c62b90f0b3e70448fe29062233`.
- Seam publica escolhida: skill `/curate [proposal-id]` com MCP sobre stdio.
- Alternativa nao escolhida para este tracer: comando interativo `gpm curate`.
- Confirmacao humana pertence ao contrato da skill; o servidor exige um preview valido e single-use, mas nao afirma conseguir provar a origem humana da confirmacao.

## Cenario implementado

```text
memory_proposal_list
-> selecionar uma proposta create
-> inspecionar conhecimento e fontes
-> escolher path em categoria configurada
-> memory_curate_preview
-> revisar memoria relacionada e diff completo
-> confirmacao explicita
-> memory_curate_apply
-> commit com pagina adicionada + proposta removida
-> memory_search em processo novo
```

## Contratos entregues

### Inbox

- Le somente propostas Markdown diretas em `proposals/` e commitadas no `HEAD` de `main`.
- Usa parser fechado para o formato emitido pelo produto; path, ID, slug, ordem dos campos, quoting e renderizacao canonica sao revalidados.
- Rejeita proposta alterada no index ou worktree depois da leitura.
- Repassa todos os campos pelo Intake Guard antes de expo-los na listagem.
- Listagem e preview nao criam state root nem alteram o repositorio.

### Preview

- Aceita somente `suggested_action: create`.
- Exige target novo sob `memories/<categoria-configurada>/**/*.md`.
- Revalida `gpm.json`, categorias, proposta, privacidade, `HEAD` e ausencia do target.
- Gera pagina com H1, page ID, secao `Overview` e section ID estavel.
- Recupera memoria canonica relacionada pelo titulo e conhecimento da proposta dentro de budget fixo.
- Retorna o diff de adicao da pagina e remocao da proposta, alem de token aleatorio mantido apenas em memoria.

### Apply e Git

- Consome o token na primeira tentativa; qualquer erro exige novo preview.
- Repete as validacoes sob o lock compartilhado com a captura.
- Constroi a tree do commit a partir de `HEAD`, nao do staging do usuario.
- Verifica que o commit possui exatamente `A <target>` e `D <proposal>`.
- Usa `update-ref` compare-and-swap sobre `main`.
- Reconcilia somente os dois paths controlados no index real.
- Preserva arquivos staged e unstaged alheios.
- Remove do worktree a proposta somente quando seus bytes ainda correspondem ao snapshot revisado.
- Nao acessa remotos nem executa hooks Git.

O commit e construido e verificado integralmente antes de `main` avancar; portanto uma interrupcao nao consegue publicar uma tree contendo apenas um dos dois paths. Recovery de index e worktree para falhas posteriores ao `update-ref` pertence a T7.

## Alteracoes de suporte

- `Memory Repository` passou a aceitar paginas em subdiretorios canonicos e a criar a cadeia de diretorios sem seguir symlinks.
- O lock do inbox foi extraido para ser compartilhado entre captura e curadoria no mesmo clone.
- O setup instala tanto `/remember` quanto `/curate` e continua idempotente para skills gerenciadas pelo produto.
- A recuperacao continua excluindo propostas e lendo somente categorias configuradas.

## Testes e evidencias

Os testes cobrem:

- caminho completo propose, list, preview e apply;
- preview sem mutacao;
- listagem sem criacao de estado privado;
- bloqueio de proposta sensivel inserida diretamente em Git antes de qualquer disclosure;
- memoria canonica relacionada no preview;
- token single-use;
- drift de `HEAD` sem criacao de memoria nem consumo da proposta;
- commit com exatamente os dois paths controlados;
- preservacao de staged e unstaged alheios;
- instalacao e contrato da skill `/curate`;
- recuperacao do conhecimento por um novo processo MCP.

Verificacao executada:

```text
make verify
```

Resultado em 2026-09-10: `go test ./...`, `go test -race ./...`, `go vet ./...` e `go build ./...` passaram com Go 1.25.12.

## Revisao

A revisao estatica foi executada nos eixos de standards e spec. Foram corrigidos antes da verificacao final:

- listagem que poderia expor conteudo inserido manualmente antes do Intake Guard;
- ausencia do conhecimento e das fontes antes da escolha do target;
- ausencia de nova consideracao da memoria canonica durante a curadoria;
- E2E que verificava somente o path, nao o conhecimento recuperado;
- nome `NewReadOnly`, que nao representava a capacidade real do objeto.

Permanecem como divida deliberada:

- helpers e parte da mecanica Git duplicados entre captura e curadoria;
- recovery de index e worktree quando uma falha ocorre depois do avanco atomico de `main`;
- update, remove, discard, lotes e referencias verificadas.

Os dois primeiros itens devem ser tratados juntos em T7 para evitar extrair prematuramente uma abstracao que ainda precisa suportar transacao multi-path e recovery.

## Criterio de conclusao

T1 esta concluida porque:

- o fluxo parte de um repositorio com `gpm.json`, uma proposta e nenhuma memoria canonica;
- preview e recusas anteriores ao apply nao escrevem memoria;
- o apply aceito publica somente pagina e consumo da proposta;
- a memoria resultante e recuperada em outro processo sem acesso a sessao original;
- staging e worktree alheios permanecem preservados;
- a matriz normativa do repositorio passa.

T1 nao conclui a Fase 3. Ela libera T2 e fornece a seam que sera expandida pelas tarefas da Fase 3B.
