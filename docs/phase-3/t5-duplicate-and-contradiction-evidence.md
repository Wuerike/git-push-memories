# T5: evidencia de duplicidade e contradicao

Status: **implementada e verificada em 2026-09-10**.

## Hipotese

Dedupe byte-exato e busca lexical sobre candidatos limitados podem evitar publicacoes identicas e tornar equivalencias ou conflitos revisaveis sem atribuir decisao semantica ao servidor.

## Ponto fixo e seam

- Ponto fixo da implementacao: `4c9cb53`.
- Seams publicas: `memory_propose` para dedupe e `/curate` + `memory_curate_preview` para evidencia e decisao.
- Autoridade duravel permanece nos arquivos Markdown e commits Git; classificacoes assistidas nao criam outro store.

## Cenarios implementados

```text
memory_propose
-> normalizar e validar payload
-> comparar conteudo Markdown canonico sob lock
-> se identico: retornar ID, path e commit existentes com duplicate=true
-> se distinto: publicar nova proposta path-scoped
```

```text
/curate
-> listar proposta selecionada
-> buscar e expandir memoria
-> classificar candidatos como equivalent, contradictory, related ou unrelated
-> contradicao exige escolha humana da resolucao
-> preview retorna diff + canonical_candidates + related_proposals
-> confirmar decisao e diff ou cancelar
```

## Contratos entregues

### Dedupe exato

- A igualdade cobre todos os campos persistidos da proposta depois da normalizacao, pela representacao Markdown byte-exata.
- A verificacao ocorre dentro do lock compartilhado do repositorio antes de gerar ID, criar arquivo ou escrever objeto Git.
- Chamadas concorrentes identicas convergem para uma proposta: uma publica e a outra retorna `duplicate: true`.
- O retorno duplicado identifica a proposta e seu commit de origem; `HEAD`, index e worktree nao mudam.
- Mudanca em qualquer byte canonico, inclusive no conhecimento ou source paths, continua criando proposta distinta.

### Evidencia de curadoria

- `canonical_candidates` usa o ranking deterministico da recuperacao sobre secoes folha atuais.
- `related_proposals` exclui a proposta selecionada, revalida schema e privacidade, ranqueia titulo + conhecimento pelo mesmo score lexical e limita o resultado a tres itens e 4096 bytes.
- Conhecimento longo de proposta e apresentado como excerpt UTF-8 limitado e marcado como truncado.
- Toda a evidencia e vinculada ao mesmo `HEAD` do target e da proposta; o preview e revalidado antes da emissao do token.
- O reason code fechado de descarte passa a incluir `duplicate` para uma equivalencia sustentada por candidato identificado.

### Classificacao e decisao

- A skill sugere uma classificacao por candidato: `equivalent`, `contradictory`, `related` ou `unrelated`.
- A classificacao e assistida e apresentada como hipotese; o GPM nao afirma equivalencia ou conflito semantico.
- Contradicao exige que a pessoa escolha entre preservar o canonico e descartar, substituir uma folha com estado completo, manter fatos separadamente escopados ou cancelar.
- Se a evidencia do preview mudar classificacao ou resolucao, a tentativa termina e um novo preview e necessario.
- Apply continua exigindo confirmacao explicita do diff e token single-use.

## Limites conhecidos da busca para a Fase 5

- O score conta apenas termos distintos compartilhados; nao existem stemming, sinonimos, embeddings ou entendimento de parafrase.
- Nao ha stopword list, portanto termos genericos podem produzir candidatos fracos.
- Somente secoes folha canonicas participam da recuperacao.
- O limite de tres propostas e os budgets podem omitir candidatos relevantes; `truncated` torna esse fato observavel.
- Ausencia de candidato nao prova novidade, e presenca nao prova duplicidade ou contradicao.

## Testes e evidencias

- proposta repetida sequencialmente retorna a mesma identidade e nao avanca `HEAD`;
- chamadas concorrentes identicas publicam apenas uma proposta;
- proposta quase igual continua distinta;
- resposta MCP expoe `duplicate: true` e o commit original;
- preview separa candidato canonico e proposta relacionada, exclui a selecionada e ignora proposta sem score;
- preview de evidencia nao escreve nem avanca `HEAD`;
- contratos das skills exigem classificacao visivel e escolha humana para contradicao.

Verificacao final:

```text
make verify
```

Resultado em 2026-09-10: `go test ./...`, `go test -race ./...`, `go vet ./...` e `go build ./...` passaram com Go 1.25.12.

## Criterio de conclusao

T5 esta concluida porque propostas byte-identicas nao sao republicadas, candidatos canonicos e do inbox aparecem separadamente no preview, contradicoes sugeridas exigem uma resolucao humana antes de qualquer apply, e as limitacoes conhecidas da busca lexical estao registradas para a Fase 5.
