# Mapa de capacidades da Fase 1

## Status da sintese

Este mapa consolida a Etapa 8 e prepara alternativas para a decisao da Etapa 9. Ele nao escolhe a base arquitetural nem autoriza incorporar codigo de terceiros.

A hipotese preferida para o ADR e uma **arquitetura propria**, com um unico contrato de Markdown, proposta e aplicacao. Permanecem abertas tres alternativas: base Go com adaptacao seletiva do Markdown do agent-memory, base Python com adaptacao seletiva de primitivas Palinode, ou implementacao limpa informada por ambos. Fork integral e runtime misto foram eliminados pelas evidencias.

O fluxo usado para comparar essas alternativas e:

```text
adapter Claude Code
-> filtro de entrada em memoria
-> proposta imutavel e revisavel
-> operacao Markdown deterministica
-> apply com revalidacao e drift
-> publicacao Git isolada
-> recuperacao textual progressiva
```

Markdown e Git sao a unica autoridade duravel. Propostas sao artefatos temporarios de mudanca, e qualquer indice local e derivado e descartavel. Daemon, store de sessao, transcript, banco vetorial, grafo e estado operacional controlado por LLM ficam fora do primeiro ciclo.

## Classificacoes

| Valor | Significado neste mapa |
|---|---|
| `adotar` | manter um desenho demonstrado sem mudanca comportamental relevante |
| `adaptar` | reutilizar fonte ou implementacao, alterando sua interface para cumprir as invariantes do produto |
| `reimplementar` | implementar o conceito independentemente dentro da arquitetura propria |
| `referenciar` | usar somente como evidencia e fonte de decisoes |
| `rejeitar` | excluir deliberadamente do primeiro ciclo |

Qualquer `adaptar` permanece condicionado ao inventario exato e ao ADR. Nenhum fork ou modulo de terceiro foi adotado nesta fase.

## Mapa principal

| Capacidade | Disposicao | Fonte determinante | Decisao para o produto |
|---|---|---|---|
| Formato e organizacao do Markdown | `adaptar` | agent-memory; Basic Memory como referencia | Usar paginas legiveis, IDs estaveis de secao e edicao byte-preserving. A disposicao so se materializa se Go e copia de fonte forem aprovados no ADR. |
| Indice local e reconstrucao | `referenciar` | Basic Memory e agent-memory | A Fase 2 comeca com arquivos e busca textual simples. FTS5 so entra depois de falha medida; qualquer implementacao futura deve ser read-only contra Markdown. |
| Contrato de proposta | `reimplementar` | agent-memory e ai-memory | Criar proposta imutavel com operacoes, hashes completos de baseline/post-state, diff, referencias de evidencia, revisao e status; nao usar staging mutavel nem DB-authoritative. |
| Consolidacao de estado atual | `reimplementar` | operacoes por secao do agent-memory; supersessao do ai-memory | Criar politica propria de substituicao deterministica do estado autoritativo. Append nao pode representar fato atual por padrao. |
| Validacao e privacidade | `reimplementar` | falha comum dos sete candidatos; twiceshy como referencia | Colocar filtro fail-closed antes da primeira persistencia de corpo ou metadado. Rejeicao nao pode bloquear Claude Code nem registrar o conteudo rejeitado. |
| Diff e aplicacao | `reimplementar` | agent-memory e spike de roots | Usar o motor Markdown apenas pela decisao de formato; criar orquestracao propria que revalida path, schema, privacidade, operacao, arquivos e hashes no apply. |
| Publicacao Git e pull request | `reimplementar` | twiceshy e claude-bedrock como referencias negativas/positivas | Publicar somente paths da proposta em branch/indice isolado; nunca consumir staging preexistente. PR fica depois do fluxo local minimo. |
| Integracao com Claude Code | `reimplementar` | claude-mem e agent-memory | Na Fase 2, criar captura explicita e recuperacao por MCP sob interfaces do produto. Hooks automaticos usam a mesma interface apenas na Fase 4. |
| Recuperacao progressiva | `reimplementar` | claude-mem; FTS do agent-memory como referencia | Retornar resumo e paths dentro do orcamento; carregar paginas integrais somente sob demanda. Nao expor sessoes ou schema SQLite. |
| Proveniencia e evidencia | `reimplementar` | Palinode | Usar source path, hash de quote, claims e relacoes `backed_by`/`contradicts` em contrato proprio. Evidencia deve acompanhar o fato atual, nao apenas proposta arquivada. |
| Primitivas de evidencia em base Python | `adaptar` | fatia offline do Palinode | Alternativa condicional: se Python for escolhido, adaptar hashing, quotes, claims e typed links da fatia inventariada; nao adotar seu executor como segunda seam. |

## Modulos candidatos

Os nomes e interfaces abaixo sao sketches independentes de runtime para comparar as tres alternativas no ADR. Eles nao fixam tipos, persistencia, protocolo ou empacotamento de producao.

### Intake Guard

Primeira seam do fluxo de captura:

```text
inspect(candidate) -> AllowedCandidate | Rejection
```

O modulo recebe dependencias, nao cria persistencia, e retorna resultado. Ele examina corpo, rationale, sources, owner, paths e demais metadados antes de qualquer proposta, lock, log, spool, indice ou objeto Git. Prompt, resposta, transcript, credencial, PII e identificador de sessao sao rejeitados ou removidos. Falha e fail-closed para captura e fail-open para a sessao Claude Code.

### Proposal Workflow

Modulo profundo para consolidacao, revisao e aplicacao:

```text
prepare(allowedCandidate) -> ProposalView
revise(proposalId, revision) -> ProposalView
reject(proposalId, reason) -> DecisionReceipt
apply(proposalId) -> ApplyReceipt | Drift
```

A implementacao esconde descoberta de alvo, politica de estado atual, operacoes por secao, validacao de evidencia, hashes completos, diff, armazenamento temporario imutavel, rescan, drift, escrita atomica e invalidacao do indice. `revise` cria uma nova revisao imutavel; a proposta inspecionada deve ser exatamente a aplicada.

Conteudo minimo da proposta:

```text
proposal_id
revision
target_paths
operations
baseline_hashes
post_state_hashes
diff
evidence_refs
created_at
status
```

Sessao, prompt, resposta, transcript e rationale privada nao pertencem ao contrato.

### Memory Repository

Modulo que protege a autoridade Markdown/Git:

```text
inspect(scope) -> MemorySnapshot
apply(changeSet) -> RepositoryReceipt
```

A implementacao possui root/path confinement, schema Markdown, IDs, parsing/rendering byte-preserving, lock, hashes completos e escrita atomica. Code roots e memory root sao distintos. Adapter Claude, Proposal Workflow e Publisher nao gravam Markdown por caminhos alternativos.

### Retrieval

Modulo que esconde busca, indice e orcamento:

```text
retrieve(query, scope, budget) -> ContextBundle
expand(handle, budget) -> ContextBundle
```

Na Fase 2, a implementacao pode percorrer Markdown diretamente. Se FTS5 for justificado, existencia, rebuild, ranking e fallback ficam internos. O chamador recebe excerpts, paths canonicos e handles de evidencia, sem conhecer SQLite.

### Publisher

Modulo executado somente depois de apply valido:

```text
publish(applyReceipt, mode) -> PublicationReceipt
```

Adapters previstos: diff local, branch Git e pull request GitHub. O primeiro tracer bullet precisa apenas de diff/revisao local e commit path-scoped. Branch/push/PR entram quando a politica da Fase 3 for definida.

### Claude Adapter

Adapter de integracao, sem autoridade sobre memoria:

```text
captureExplicit(text, sourceHints) -> ProposalView
retrieveContext(query, budget) -> ContextBundle
```

Ele chama Intake Guard, Proposal Workflow e Retrieval. Nao cria store, indice, proposta ou publicacao paralelos. Hooks futuros devem atravessar a mesma interface.

## agent-memory versus Palinode

Os dois candidatos nao podem controlar a mesma seam. agent-memory usa `<!-- @id: ... -->`, snapshots e operacoes por secao; Palinode usa `<!-- fact:... -->`, frontmatter epistemico e outro vocabulario de operacoes. Combina-los criaria duas identidades, dois parsers, duas regras de drift e dois ciclos de historico.

| Seam | agent-memory | Palinode | Owner recomendado |
|---|---|---|---|
| parsing/rendering Markdown | byte-preserving por secao | frontmatter e facts | Memory Repository, informado por agent-memory |
| operacoes deterministicas | replace/archive/delete por secao | `KEEP`, `UPDATE`, `MERGE`, `SUPERSEDE` e outros | Proposal Workflow, com semantica unica baseada no estado atual |
| roots, lock e concorrencia | demonstrados no spike apos adaptacao | facade serializada | Memory Repository, informado por agent-memory |
| baseline e drift | correcoes demonstradas no spike | guarda adicionada pela facade | Proposal Workflow, contrato proprio |
| proposta e revisao | staging/diff funcional, mas inseguro | preview nao reaplicavel no produto completo | Proposal Workflow, reimplementado |
| indice e rebuild | SQLite/FTS5 | excluido da fatia offline | Retrieval; adiar FTS |
| quotes, claims e relacoes | proveniencia limitada | primitivas deterministicas | contrato proprio informado por Palinode |
| Git | staging/commit incompletos | best-effort no produto completo | Publisher proprio |

Regra de ownership: **agent-memory informa como o Markdown muda; Palinode informa por que a mudanca e justificavel**.

## Exclusoes

| Item | Decisao | Motivo |
|---|---|---|
| fork integral do agent-memory | `rejeitar` | roots exigiram prototipo; privacidade, sessoes, staging/apply e Git permanecem incompativeis |
| runtime Palinode ao lado de Go | `rejeitar` | segundo runtime e segundo modelo de operacoes sem leverage proporcional |
| executor Palinode junto ao motor agent-memory | `rejeitar` | semanticas concorrentes e evidencia nao transferida em `UPDATE` |
| daemon, worker ou backend obrigatorio | `rejeitar` | viola operacao local minima |
| persistencia de sessao, prompt, resposta ou transcript | `rejeitar` | viola a privacidade canonica |
| vector DB, grafo e busca semantica na Fase 2 | `rejeitar` | roadmap exige falha medida da busca textual primeiro |
| auto-merge ou apply dirigido apenas pelo modelo | `rejeitar` | revisao e aceite devem ser verificaveis |

## Comparacao de alternativas

| Alternativa | Custo inicial | Dependencias | Risco de manutencao | Conclusao |
|---|---|---|---|---|
| fork agent-memory | medio; alto apos hardening | Go, Goldmark, flock, YAML, modernc SQLite, MCP/Cobra | alto; herda arquitetura rejeitada | rejeitar |
| base propria Go com Markdown agent-memory adaptado | medio | Go e Goldmark inicialmente; Git externo | medio; fonte copiada precisa tracking | recomendacao se Go for escolhido |
| base propria Python com primitivas Palinode | medio | Python, python-frontmatter, PyYAML e Git | medio-alto; interfaces beta e adapter necessario | alternativa se Python for escolhido |
| Go mais runtime Palinode Python | alto | duas toolchains e dois modelos | muito alto | rejeitar |
| implementacao limpa informada pelos dois | medio-alto | somente dependencias escolhidas pelo produto | menor acoplamento externo | alternativa forte ao source adaptation |

O indice `modernc.org/sqlite` deve ser adiado: sua closure e grande e o roadmap so pede FTS apos falha medida. Busca textual por arquivos e metadados basta para o primeiro tracer bullet.

## Referencias por candidato

| Candidato | Papel final da Fase 1 |
|---|---|
| Basic Memory | referencia para Markdown, reconciliacao arquivo-indice, CAS e busca local; AGPL impede copia |
| ai-memory | reimplementacao conceitual de supersessao, baseline/drift, wiki separada e sanitizacao tipada |
| Palinode | referencia principal para evidencia; source adaptation apenas se Python for a base unica |
| claude-mem | referencia para hooks, IO compacto, fail-open e recuperacao progressiva |
| claude-bedrock | referencia para multiplos vaults, proveniencia e estrategias Git; nenhum modulo deterministico escolhido |
| agent-memory | candidato a adaptacao seletiva do Markdown; restante informa operacoes, diff, lock e FTS |
| twiceshy | referencia para quarantine, estados de confianca, counter-evidence e recuperacao precision-first; AGPL impede copia |

## Gates para o ADR

A Etapa 9 deve decidir:

1. runtime principal: Go, Python ou implementacao limpa em outro runtime;
2. source adaptation versus reimplementacao independente;
3. lista exata de arquivos copiados e politica de tracking upstream;
4. um unico modelo de IDs e schema Markdown;
5. schema e serializacao da proposta;
6. persistencia temporaria das propostas sanitizadas;
7. semantica de atomicidade e recuperacao multi-arquivo;
8. modo padrao de publicacao Git;
9. forma de empacotar licencas, notices e marcacoes de modificacao.

Esses pontos nao exigem novos testes nos candidatos. Sao escolhas da arquitetura do produto a serem registradas no ADR.

## Entrada para a Fase 2

O primeiro fluxo deve ser deliberadamente menor que o mapa completo:

```text
captura explicita
-> Intake Guard em memoria
-> uma proposta de replace de secao
-> diff local
-> aceite explicito e apply com drift
-> commit path-scoped
-> recuperacao textual em processo novo
```

Captura automatica, pull request obrigatorio, contradicao por agente, FTS5 e busca semantica ficam nas fases posteriores definidas pelo roadmap.
