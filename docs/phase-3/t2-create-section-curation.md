# T2: curadoria de proposta create para secao existente

Status: **implementada e verificada em 2026-09-10**.

## Hipotese

Uma proposta `create` relacionada a uma pagina canonica existente pode acrescentar uma secao sem criar pagina duplicada, reformatar conteudo anterior ou consumir alteracoes alheias.

## Ponto fixo e seam

- Ponto fixo da implementacao: `fba16a8`.
- Seam publica: skill `/curate [proposal-id]` com MCP sobre stdio.
- A operacao escolhida no preview passou a ser explicita: `create_page` ou `create_section`.
- A proposta continua declarando apenas uma acao sugerida; a operacao canonica pertence a curadoria.

## Cenario implementado

```text
memory_proposal_list
-> selecionar proposta create
-> memory_search + memory_expand
-> escolher create_section, pagina e heading
-> memory_curate_preview
-> revisar candidatos relacionados e diff completo
-> confirmacao explicita
-> memory_curate_apply
-> commit com pagina modificada + proposta removida
-> memory_search encontra a nova secao
```

## Contratos entregues

### Skill

- `/curate` recebe acesso read-only a `memory_search` e `memory_expand` para decidir se o conhecimento pertence a uma pagina existente.
- A busca ocorre antes da escolha entre `create_page` e `create_section`.
- Busca e expansao usam budget de 4096 bytes; budget insuficiente encerra a tentativa sem preview ou escrita.
- A skill interrompe o fluxo quando o conhecimento ja esta representado ou exige update, que pertence a T3.
- `heading` e enviado somente para `create_section`.
- Qualquer mudanca depois do preview exige uma nova invocacao explicita.

### Preview

- `create_section` exige target existente dentro de categoria configurada e heading novo.
- O Memory Repository valida schema, limpeza contra `HEAD`, heading, ID derivado e colisao.
- O post-state acrescenta uma secao H2 com ID `<page-id>-<heading-slug>`.
- O diff combina a alteracao byte-preserving da pagina e a remocao completa da proposta.
- Preview nao altera pagina, proposta, `HEAD` ou index.

### Apply e Git

- Apply recalcula a mesma operacao, baseline, section ID, post-state e diff.
- Baseline usa hash do arquivo, `HEAD` e blob Git da pagina existente.
- O commit esperado possui exatamente `M <target>` e `D <proposal>`.
- O index real parte dos bytes existentes e reconcilia somente target e proposta.
- Falha no compare-and-swap de `main` tenta restaurar o baseline quando o worktree ainda corresponde ao post-state aplicado.
- O fluxo preserva staging e worktree alheios e nao acessa remotos.

## Testes e evidencias

Os testes focados cobrem:

- proposta `create` curada para nova secao em pagina existente;
- preview sem mutacao;
- section ID derivado e conteudo final exato;
- bytes preexistentes preservados antes da secao anexada;
- commit com exatamente pagina modificada e proposta removida;
- proposta consumida e nova secao recuperavel;
- heading existente rejeitado com `invalid_target`, sem alterar pagina, proposta ou `HEAD`;
- regressao do fluxo `create_page` da T1;
- contrato da skill com busca, expansao, preview, confirmacao e apply.

Comando focado principal:

```text
go test ./internal/mcpserver -run TestCurateCreateSectionUpdatesExistingPageAndConsumesProposal -count=1
```

Verificacao final:

```text
make verify
```

Resultado em 2026-09-10: `go test ./...`, `go test -race ./...`, `go vet ./...` e `go build ./...` passaram com Go 1.25.12.

## Limites mantidos

- `update`, `remove`, descarte e lotes continuam indisponiveis.
- Similaridade lexical orienta a escolha, mas nao decide equivalencia ou contradicao.
- Referencias continuam nao verificadas quanto a existencia e sustentacao.
- Recovery depois de `update-ref` e deduplicacao da mecanica Git permanecem para T7.

## Criterio de conclusao

T2 esta concluida porque a matriz `make verify` passou, o fluxo cria e recupera a secao sem alterar bytes anteriores, e falhas de colisao permanecem sem escrita. Isso libera T3, que usara identidade precisa de secao para substituir conhecimento existente.
