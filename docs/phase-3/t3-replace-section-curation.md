# T3: curadoria de proposta update para substituicao de secao

Status: **implementada e verificada em 2026-09-10**.

## Hipotese

Uma proposta `update` pode substituir somente o corpo de uma secao folha e manter o estado atual como autoritativo, sem alterar headings, anchors, outras secoes ou staging alheio.

## Ponto fixo e seam

- Ponto fixo da implementacao: `f8c5e17`.
- Seam publica: skill `/curate [proposal-id]` com MCP sobre stdio.
- Operacao canonica adicionada: `replace_section`.
- Identidade do target: path canonico e section ID obtidos de um handle expandido no estado atual.
- O path sugerido na proposta orienta a busca, mas nao substitui a selecao e revalidacao da secao.

## Cenario implementado

```text
memory_proposal_list
-> selecionar proposta update
-> memory_search + memory_expand
-> escolher uma secao folha sem ambiguidade
-> construir replacement completo preservando fatos validos
-> memory_curate_preview com path + section_id + content_hash
-> revisar estado relacionado e diff completo
-> confirmacao explicita
-> memory_curate_apply
-> commit com pagina modificada + proposta removida
-> memory_search retorna somente o estado novo
```

## Contratos entregues

### Skill

- `/curate` aceita propostas `create` e `update`; `remove` permanece indisponivel.
- Para `update`, a skill escolhe `replace_section` somente depois de expandir o target folha.
- A proposta fornece o fato novo; a skill combina esse fato com o corpo expandido e produz um replacement completo, preservando todo conhecimento ainda valido.
- Target ausente, nao folha, ambiguo ou que exigiria alterar estrutura encerra o fluxo.
- `section_id`, `content_hash` observado e `replacement` sao enviados somente para `replace_section`; `heading` continua exclusivo de `create_section`.

### Preview

- Exige `suggested_action: update`, target em categoria configurada, section ID, content hash observado e replacement completo.
- O Memory Repository confirma que a secao existe, e folha e pertence ao blob atual da pagina.
- O content hash deve corresponder ao handle expandido; selecao obsoleta retorna drift em vez de rebasing silencioso.
- O replacement completo passa novamente pelo Intake Guard antes do preview.
- Headings, anchors e a forma ordenada do documento permanecem identicos.
- Baseline inclui bytes, hash do arquivo, `HEAD`, blob e modo Git.
- Diff combina a substituicao byte-preserving e a remocao da proposta.

### Apply e Git

- Recalcula a substituicao a partir do blob de baseline e compara post-state e diff revisados.
- Revalida proposta, privacidade, configuracao, `HEAD`, target e section ID sob lock.
- Usa o mesmo commit multi-path da curadoria de secao, com exatamente `M <target>` e `D <proposal>`.
- Preserva modo Git, index e worktree alheios.
- Falha anterior ao avanco de `main` reconcilia baseline ou retorna drift sem esconder erro de rollback.

## Testes e evidencias

Os testes focados cobrem:

- proposta `update` aplicada a secao folha;
- preview sem mutacao;
- diff removendo o estado anterior e adicionando o novo;
- fato novo incorporado sem apagar ownership ainda valido da secao;
- heading e anchor preservados;
- arquivo final exato sem manter timeout obsoleto;
- commit com exatamente pagina modificada e proposta removida;
- proposta consumida e estado novo recuperavel;
- secao nao folha rejeitada com `invalid_target`, sem alterar pagina, proposta ou `HEAD`;
- content hash obsoleto rejeitado com `curation_drift`, sem alteracao;
- regressao dos fluxos `create_page` e `create_section`;
- contrato da skill para operacoes e campos mutuamente exclusivos.

Comando focado principal:

```text
go test ./internal/mcpserver -run TestCurateReplaceSectionUpdatesCurrentStateAndConsumesProposal -count=1
```

Verificacao final:

```text
make verify
```

Resultado em 2026-09-10: `go test ./...`, `go test -race ./...`, `go vet ./...` e `go build ./...` passaram com Go 1.25.12.

## Limites mantidos

- A proposta Markdown v1 ainda sugere apenas o path; a identidade precisa e escolhida na curadoria.
- Nao existe edicao de heading, anchor ou secao nao folha.
- `remove`, descarte e lotes continuam indisponiveis.
- Contradicao sugerida nao escolhe automaticamente qual estado deve vencer.
- Referencias continuam nao verificadas e recovery posterior ao `update-ref` permanece para T7.
- Branches repetidos por operacao no coordenador de curadoria devem ser aprofundados antes de T4/T7 se a proxima operacao aumentar shotgun surgery.

## Criterio de conclusao

T3 esta concluida porque a matriz `make verify` passou, o replacement preserva fatos validos, a selecao esta vinculada ao content hash observado e somente o corpo da secao folha muda. Isso libera T4, que reutilizara a mesma identidade para remover conhecimento ou descartar uma proposta.
