# Base de implementacao propria em Go

Status: accepted

Data: 2026-09-05

O git-push-memories sera uma implementacao propria em Go, com Markdown e Git como unica autoridade duravel. Somente o parser e as primitivas de edicao byte-preserving do agent-memory serao adaptados; o fluxo de captura, privacidade, proposta, consolidacao, apply, publicacao e recuperacao sera implementado sob contratos proprios. Essa escolha preserva um unico runtime local, reutiliza o componente mais bem delimitado pelos spikes e evita herdar stores de sessao, indices autoritativos e modelos de operacao incompativeis.

## Contexto e restricoes

O produto deve transformar conhecimento descoberto em uma sessao em estado atual revisavel e recuperavel por sessoes futuras. O primeiro ciclo deve funcionar localmente para um desenvolvedor, contra um repositorio de memoria ja clonado, sem backend obrigatorio e sem persistir sessoes, prompts ou respostas.

As [avaliacoes da Fase 1](../phase-1/comparison-matrix.md) nao encontraram um candidato que atendesse ao fluxo completo. Todos falharam em privacidade antes da primeira persistencia e em nao persistir sessoes. O [mapa de capacidades](../phase-1/capability-map.md) tambem mostrou que combinar agent-memory e Palinode criaria duas identidades Markdown, dois parsers, dois modelos de operacao e dois ciclos de historico.

## Decisao

### Base arquitetural

- Usar Go como unico runtime do produto e produzir um executavel local.
- Manter o repositorio de memoria, composto por Markdown e historico Git, como unica autoridade duravel.
- Implementar os modulos Intake Guard, Proposal Workflow, Memory Repository, Retrieval, Publisher e Claude Adapter sob contratos do produto.
- Nao incorporar banco, daemon, worker, store de sessao, transcript, grafo, banco vetorial ou runtime Python no primeiro ciclo.
- Comecar a recuperacao percorrendo arquivos e usando busca textual. Qualquer indice futuro sera derivado e reconstruivel.

### Componente adaptado

Copiar e adaptar somente estes arquivos de `xChuCx/agent-memory@51dac9bf6e72c67f6314ee3abb00515986a3fa4a`:

```text
internal/markdown/parse.go
internal/markdown/splice.go
internal/markdown/section_id.go
```

Os arquivos serao movidos para um pacote do produto, receberao testes contra o schema escolhido e serao modificados apenas por interfaces do Memory Repository. A adaptacao removera a tolerancia upstream de linha vazia antes do anchor e rejeitara IDs preexistentes duplicados. A dependencia inicialmente aprovada e `github.com/yuin/goldmark@v1.7.4`.

Dois arquivos da shortlist da Etapa 8 nao serao incorporados:

- `validate.go`, porque sua verificacao best-effort nao substitui validacao estrutural e de privacidade;
- `merge.go`, porque merge entre branches esta fora da Fase 2 e sua politica de conservar exclusoes unilaterais nao foi escolhida para o produto.

Nenhum arquivo do Palinode sera copiado. Claims, quote hashes e relacoes `backed_by` e `contradicts` serao reimplementados somente quando entrarem em um fluxo da arquitetura propria.

### Contratos iniciais

| Tema | Decisao |
|---|---|
| identidade Markdown | uma secao e identificada pelo par `canonical_relative_path + section_id` |
| section ID | comentario `<!-- @id: slug -->` imediatamente depois do heading; unico dentro do arquivo e estavel depois de criado |
| schema Markdown | paginas legiveis organizadas por headings e IDs; sem frontmatter obrigatorio na Fase 2; regras por categoria pertencem ao Memory Repository |
| proposta | documento JSON versionado, sanitizado e imutavel por revisao, contendo somente os campos definidos no mapa de capacidades |
| persistencia da proposta | arquivo local temporario, fora do repositorio de memoria, criado com permissao `0600` por escrita atomica; nenhuma proposta existe antes do Intake Guard aprovar todo o payload |
| ciclo da proposta | revisao cria novo documento; rejeicao ou apply remove o payload e conserva apenas um receipt sem conteudo de sessao |
| drift | hashes SHA-256 do arquivo completo no baseline e no post-state; apply recalcula e rejeita qualquer divergencia |
| atomicidade | a Fase 2 aceita uma proposta para exatamente um arquivo; propostas multi-arquivo sao rejeitadas antes de persistir; escrita usa arquivo temporario adjacente, `fsync`, rename e `fsync` do diretorio |
| publicacao Git | commit local depois de aceite explicito, limitado aos paths do ApplyReceipt e criado com indice Git temporario; staging preexistente nunca e consumido |
| pull request | adiado ate a politica de governanca da Fase 3 |

O formato JSON exato, nomes de comandos e tipos Go podem evoluir durante o tracer bullet, desde que preservem essas invariantes. Mudancas no modelo de identidade, na autoridade duravel ou na fronteira de privacidade exigem novo ADR.

### Recuperacao apos interrupcao

O apply de um arquivo segue estes pontos de durabilidade:

1. A proposta sanitizada so se torna disponivel depois de escrita atomica, `fsync` do arquivo e `fsync` de seu diretorio.
2. Sob lock do Memory Repository, o apply revalida schema, privacidade, path e baseline, escreve o novo Markdown em arquivo adjacente, executa `fsync`, rename e `fsync` do diretorio.
3. O rename duravel e o ponto de commit do apply. Em seguida, um ApplyReceipt e persistido atomicamente antes da remocao do payload da proposta.
4. Se houver interrupcao antes do rename, o baseline continua autoritativo e o apply pode ser repetido. Temporarios controlados e incompletos sao descartados na proxima inicializacao.
5. Se houver interrupcao depois do rename e antes do receipt, a recuperacao compara o arquivo atual ao `post_state_hash`: correspondencia reconstrui o receipt sem reaplicar; correspondencia apenas com o baseline permite repetir; qualquer outro estado retorna drift.
6. Publicacao Git e repetivel a partir do ApplyReceipt. Falha ou interrupcao no commit deixa o estado `applied_unpublished` e nao desfaz o Markdown ja aplicado.

O payload da proposta so e removido depois de um receipt terminal duravel. Rejeicao tambem cria seu receipt antes da remocao. Assim, uma interrupcao nao transforma uma proposta desconhecida em uma nova alteracao nem perde a informacao necessaria para reconciliar o estado.

## Evidencias determinantes

- O [spike de roots do agent-memory](../phase-1/spikes/agent-memory-split-roots.md) demonstrou operacao offline, roots separados, edicao Markdown, concorrencia, drift e reconstrucao, mas precisou corrigir cold start, IDs de staging e perda de atualizacao.
- O pacote Markdown do agent-memory opera por offsets e preserva bytes fora das secoes alteradas. Ele e menor e mais isolado que a orquestracao, o staging, o indice e a integracao Git do candidato.
- O [spike Palinode](../phase-1/spikes/palinode-offline-extraction.md) demonstrou primitivas de evidencia offline, mas a fatia nao e uma API estavel, depende de Python e exige adapter para remover integracoes do produto completo.
- A [analise de licencas](../phase-1/licenses.md) permite adaptar o componente Apache-2.0 do agent-memory e Goldmark MIT mediante as obrigacoes de redistribuicao registradas abaixo. Basic Memory e twiceshy permanecem referencias sem copia porque suas condicoes AGPL sao incompativeis com reutilizacao direta mantendo a distribuicao do produto sob Apache-2.0 sem relicenciamento explicito.
- A busca textual e suficiente para o primeiro tracer bullet; FTS5 e sua closure nao possuem beneficio medido nesta fase.

## Capacidades por referencia

| Referencia | Modalidade final | Capacidade aproveitada |
|---|---|---|
| Basic Memory | `referenciar` | Markdown legivel, reconciliacao de indice derivado, CAS e busca local |
| ai-memory | `reimplementar` | supersessao, baseline/drift, wiki separada e sanitizacao tipada |
| Palinode | `reimplementar` | claims, quote hashes, proveniencia e relacoes tipadas |
| claude-mem | `referenciar` | integracao Claude, fail-open e recuperacao progressiva |
| claude-bedrock | `referenciar` | multiplos vaults, proveniencia e estrategias Git |
| agent-memory | `adaptar` | parsing, IDs e edicao Markdown byte-preserving nos tres arquivos aprovados |
| twiceshy | `referenciar` | quarentena, estados de confianca, counter-evidence e recuperacao precision-first |

Nenhum candidato e adotado como base ou fork. `Reimplementar` significa implementar o conceito sem copiar sua expressao; `referenciar` significa usar somente a evidencia para orientar contratos e testes.

## Alternativas rejeitadas

### Fork integral do agent-memory

Rejeitado porque o candidato fornece capacidades uteis, mas sua persistencia de sessoes, fronteira de privacidade, staging mutavel, apply, uso do indice Git e acoplamento entre roots nao atendem as invariantes. Corrigi-lo exigiria substituir a maior parte do fluxo que justificaria um fork.

### Base Python com componente Palinode

Rejeitada porque o valor diferencial do Palinode esta nas primitivas de evidencia, nao em um fluxo end-to-end compativel. Adota-la acrescentaria dependencias e uma API beta para capacidades que nao sao necessarias no tracer bullet e podem ser implementadas sob o contrato proprio quando justificadas.

### Implementacao completamente limpa

Rejeitada para o motor Markdown porque repetiria o risco ja resolvido pelo parser e pelos splices byte-preserving demonstrados. Ela continua sendo a regra para todas as demais capacidades.

### Runtime misto Go e Python

Rejeitado porque duplica empacotamento, modelos de operacao e superficies de falha sem oferecer leverage proporcional. O executor Palinode nao sera uma segunda seam de aplicacao.

### Forks ou componentes dos demais candidatos

Rejeitados por incompatibilidade de licenca, backend obrigatorio, stores de sessao, falta de isolamento ou ausencia de um componente deterministico que alterasse a decisao. Seus comportamentos permanecem referencias conforme o mapa de capacidades.

## Consequencias

- O produto assume responsabilidade por privacidade, schema, proposta, consolidacao, atomicidade, Git e integracao Claude Code.
- A adaptacao de fonte reduz o risco de corromper Markdown, mas cria uma obrigacao de rastrear origem e modificacoes.
- O primeiro tracer bullet fica deliberadamente limitado a uma operacao `replace` sobre uma secao de um arquivo.
- Multi-arquivo, merge driver, branch, push, pull request, hooks automaticos e indice local nao fazem parte da Fase 2.
- Evidencia epistemica sera representada por contratos proprios; a implementacao pode ser adiada ate existir um caso de uso que a exija.

## Licencas e manutencao upstream

Na mesma mudanca que incorporar pela primeira vez o componente adaptado:

- incluir o texto Apache-2.0 do agent-memory e preservar o copyright de Teterichev Anton;
- marcar os arquivos adaptados e registrar a revisao e os blobs de origem;
- incluir o texto MIT e o copyright aplicavel do Goldmark;
- criar um inventario de terceiros distribuido com o produto;
- revisar atualizacoes upstream manualmente, sem merge ou sincronizacao automatica.

Esses itens sao criterios de aceite da incorporacao, nao decisoes arquiteturais pendentes da Fase 1.

Nenhuma obrigacao Palinode, python-frontmatter ou PyYAML entra na distribuicao enquanto nenhum fonte ou dependencia desses projetos for incorporado.

## Implicacoes para a Fase 2

O fluxo vertical minimo sera:

```text
captura explicita pelo Claude Adapter
-> Intake Guard em memoria
-> proposta temporaria de replace de secao
-> diff e aceite local
-> apply de um arquivo com verificacao de drift
-> commit path-scoped com indice isolado
-> recuperacao textual em um novo processo
```

O teste de ponta a ponta deve provar que o conhecimento publicado melhora uma tarefa posterior sem acesso a sessao original. Esse resultado, e nao a expansao do mapa completo, e o gate da Fase 2.
