# Comparação dos mecanismos de geração de memória

## Resposta executiva

- **Existe diferença provável na qualidade da memória gerada**, mas não existe um vencedor único em todos os critérios. Cada projeto otimiza uma combinação diferente de cobertura, precisão, riqueza, atualização, evidência e controle de ruído.
- **O maior ganho de cobertura está nos sistemas com hooks.** `ai-memory`, `claude-mem`, `Palinode`, `Basic Memory` e `twiceshy` conseguem capturar sessões ou eventos sem depender de um `/remember` por fato. Isso reduz esquecimentos de captura, mas aumenta o volume de material irrelevante e a superfície para interpretações erradas do modelo.
- **A maior garantia de correção técnica é oferecida pelo `twiceshy`, dentro de seu domínio estreito.** Uma experiência de engenharia nasce em quarentena e pode exigir reprodução executável em sandbox e julgamento adicional antes de ser servida como validada. Essa qualidade não se generaliza para memória ampla de domínio, pessoas ou decisões.
- **A memória mais rica não é necessariamente a mais confiável.** `claude-mem`, `ai-memory` e `Palinode` dão ao modelo mais sessão, eventos, relações e memória anterior. Isso favorece contexto e recall, mas aumenta dependência do modelo, risco de ruído e custo de resolver fatos contraditórios.
- **Não é possível afirmar superioridade empírica global sem um benchmark comum.** As conclusões desta análise são inferências arquiteturais a partir dos mecanismos atuais. Benchmarks publicados por projetos diferentes usam corpora, tarefas, modelos e métricas diferentes e não permitem uma comparação direta de qualidade.

## Escopo e método

Foram analisados os sete projetos classificados como referências, além do projeto atual:

- `git-push-memories`;
- `basic-memory`;
- `ai-memory`, de `akitaonrails`;
- `Palinode`;
- `claude-mem`;
- `claude-bedrock`;
- `agent-memory`;
- `twiceshy`.

A análise seguiu o código-fonte, prompts, skills, schemas e documentação primária presentes em cada clone. Os projetos não foram executados. Para cada mecanismo foram observados:

- o gatilho de captura e a unidade de entrada;
- quem seleciona e redige o conteúdo;
- qual contexto alimenta o gerador;
- como duplicidade, contradição e mudança de estado são tratadas;
- quais validações, evidências e revisões existem;
- onde termina um registro bruto, uma proposta e uma memória canônica;
- as consequências prováveis para cobertura, precisão, riqueza, atualidade e ruído.

“Qualidade” neste documento possui seis dimensões independentes:

- **Cobertura:** probabilidade de um conhecimento útil ser capturado.
- **Precisão:** probabilidade de o conteúdo salvo representar corretamente o que ocorreu.
- **Riqueza:** quantidade de contexto útil, como causa, rationale, evidência, relações e próximos passos.
- **Consistência temporal:** capacidade de distinguir estado atual, histórico, contradição e supersessão.
- **Rastreabilidade:** capacidade de descobrir origem, transformação, modelo, revisão e histórico de uma afirmação.
- **Sinal/ruído:** proporção entre conhecimento reutilizável e material incidental.

## Revisões de referência

As descrições dos projetos externos usam estas revisões:

- **Basic Memory:** `3452c821d76c083823d020984d71e06904a1ff1e`.
- **ai-memory:** `55fcce35d2bf3e52d933f83cb651b380fd67e81b`.
- **Palinode:** `42f726921dae5493821480e913d7c52cbb046c6d`.
- **claude-mem:** `8bc631a71a487424b866756e43a6efa4574cc66b`.
- **claude-bedrock:** `7cac16ceb6064b7fbb4ec45c3ad4be31b9d9c9f0`.
- **agent-memory:** `e42f455865538a59110c6510ae8e340969feb810`.
- **twiceshy:** `aa8f1d88b23ce26b9add249ee2f507d350aca12c`.

## Basic Memory

### Gatilhos e entradas

Basic Memory combina mecanismos diferentes, cuja qualidade não deve ser confundida:

- Hooks `SessionStart` e `PreCompact` oferecem continuidade e checkpoints.
- `/bm-remember` grava uma lembrança deliberada, próxima das palavras do usuário.
- `/bm-decide` gera uma decisão com contexto, rationale, alternativas e consequências.
- `/bm-checkpoint` produz um registro rico de estado de trabalho, Git, mudanças, verificações, decisões, blockers e próximo passo.
- Skills de `memory-capture`, `memory-curate`, `memory-reflect` e `memory-defrag` orientam atualização e manutenção, mas dependem de um agente ou agendamento externo; não são um consolidador autônomo do core.

### Quem gera e qual contexto usa

Nos fluxos deliberados, o próprio coding agent formula o Markdown usando a conversa, o trabalho realizado, Git, testes e notas encontradas por busca. Não existe um LLM interno obrigatório no caminho padrão de escrita.

O hook automático de `PreCompact` do Claude é diferente: ele gera uma nota deterministicamente a partir do transcript. A implementação retém a primeira mensagem do usuário e as três últimas mensagens do usuário, além de metadados Git verificáveis. Ela não sintetiza as respostas do assistente, resultados de testes ou conclusões técnicas. Assim, o checkpoint automático é factual e barato, mas superficial.

Para Codex, o checkpoint posterior à compactação depende do agente obedecer à instrução `bm-checkpoint` usando o resumo preservado pelo host, o que pode produzir conteúdo mais rico, mas acrescenta dependência do modelo e da qualidade da compactação.

### Pipeline e destino

`write_note` grava diretamente uma nota Markdown canônica. Não há proposta obrigatória entre a redação e a memória. O writer protege conflitos de path e exige overwrite explícito; depois de criar uma nota, pode devolver similares como aviso, mas não faz merge automático.

O hook `captureEvents`, habilitado por padrão para algumas integrações, não deve ser tratado como memória de conhecimento. Ele grava envelopes de trace com metadados limitados em um WAL local. O próprio contrato afirma que envelopes são trace, não memória, e eles nunca são promovidos automaticamente ao grafo.

Markdown é a fonte do conteúdo e SQLite é índice derivado. Existe uma nuance operacional: o runtime pode aceitar conteúdo primeiro no banco e materializar o arquivo em seguida, embora o desenho declare o Markdown como fonte de verdade.

### Duplicidade, consolidação e temporalidade

Basic Memory delega grande parte da consistência semântica às skills. `memory-capture` instrui o agente a encontrar uma nota por `thread_id` ou tópico, reescrever a mesma página, substituir decisões superadas e remover contexto obsoleto. `memory-curate` e `memory-defrag` orientam merges e limpeza com confirmação.

Isso pode manter uma memória de estado atual melhor que append-only, mas a garantia depende de o agente escolher e executar a skill correta. O core não impede notas semanticamente duplicadas e não detecta contradições automaticamente.

O modelo temporal é expressivo: uma observação pode representar quando algo era verdadeiro no mundo, e datas relativas não reproduzíveis são recusadas. Essa capacidade melhora respostas históricas quando os autores fornecem os intervalos corretos.

### Validação, revisão e privacidade

Os schemas tipados ajudam buscas e briefings, mas a validação automática de writes não é obrigatória em todos os caminhos. Uma nota incompleta pode ser gravada e deixar de aparecer numa recuperação estruturada.

As skills deliberadas possuem bons critérios de qualidade. Por exemplo, `bm-checkpoint` manda registrar somente testes efetivamente executados e proíbe declarar sucesso sem evidência. Porém, isso continua sendo uma instrução ao modelo, não uma verificação pelo writer.

Não foi encontrado um scanner geral de segredos no corpo de toda nota. A captura ambiental de eventos é relativamente restrita por guardar metadados, mas uma lembrança ou checkpoint redigido pelo agente pode persistir conteúdo sensível.

Não existe confirmação humana intermediária obrigatória. O agente chama `write_note` e altera a memória canônica. A revisão acontece pela interação normal com o agente, pelo arquivo e por operações posteriores de curadoria.

### Qualidade provável

- **Cobertura média:** checkpoints e comportamento proativo ajudam, mas a memória rica ainda depende de skills e disciplina do agente.
- **Precisão alta nos fluxos deliberados bem executados:** especialmente `bm-checkpoint` e `bm-decide`, que pedem evidência e rationale.
- **Riqueza alta manualmente e baixa no checkpoint automático do Claude:** são dois perfis bastante diferentes dentro do mesmo projeto.
- **Consistência temporal boa:** há reescrita orientada a estado atual e representação temporal, mas dedupe e supersessão dependem do agente.
- **Rastreabilidade média:** Markdown, permalinks e Git ajudam; não há sempre uma proposta ou prova estruturada anterior à escrita.
- **Ruído geralmente baixo, com risco de proliferação de checkpoints:** títulos com timestamp tendem a gerar páginas novas a cada compactação.

### Fontes primárias

- [Hooks do plugin Claude Code](https://github.com/basicmachines-co/basic-memory/blob/3452c821d76c083823d020984d71e06904a1ff1e/plugins/claude-code/hooks/hooks.json#L1-L29)
- [`PreCompact` e geração do checkpoint](https://github.com/basicmachines-co/basic-memory/blob/3452c821d76c083823d020984d71e06904a1ff1e/src/basic_memory/cli/commands/hook.py#L908-L951)
- [Construção determinística da nota](https://github.com/basicmachines-co/basic-memory/blob/3452c821d76c083823d020984d71e06904a1ff1e/src/basic_memory/cli/commands/hook.py#L1048-L1172)
- [`bm-remember`](https://github.com/basicmachines-co/basic-memory/blob/3452c821d76c083823d020984d71e06904a1ff1e/plugins/claude-code/skills/bm-remember/SKILL.md#L27-L56)
- [`bm-checkpoint`](https://github.com/basicmachines-co/basic-memory/blob/3452c821d76c083823d020984d71e06904a1ff1e/plugins/claude-code/skills/bm-checkpoint/SKILL.md#L31-L123)
- [`memory-capture`](https://github.com/basicmachines-co/basic-memory/blob/3452c821d76c083823d020984d71e06904a1ff1e/skills/memory-capture/SKILL.md#L28-L75)
- [`write_note` e similares](https://github.com/basicmachines-co/basic-memory/blob/3452c821d76c083823d020984d71e06904a1ff1e/src/basic_memory/mcp/tools/write_note.py#L448-L499)
- [Modelo temporal](https://github.com/basicmachines-co/basic-memory/blob/3452c821d76c083823d020984d71e06904a1ff1e/src/basic_memory/temporal.py#L1-L43)

## ai-memory

### Gatilhos e entradas

`ai-memory` captura uma sessão de coding agent com hooks para início, prompts, antes e depois de ferramentas, compactação, parada, fim de sessão e subagentes. A unidade bruta é uma observação tipada associada a uma sessão, não uma nota já curada.

O fluxo principal separa três camadas:

- eventos recebidos pelos hooks;
- observações sanitizadas em SQLite;
- páginas Markdown produzidas por síntese determinística ou consolidação LLM.

Há ainda um loop de auto-improvement que revisa sessões anteriores e propõe novas páginas, regras ou procedimentos.

### Quem gera e qual contexto usa

Sem LLM configurado, um sintetizador determinístico cria uma página estável de sessão. Ele registra título derivado do primeiro prompt, contagens, timestamps e até 500 títulos de observações. É rastreável, mas não inclui todo o corpo de prompts e outputs; portanto captura “o que aconteceu” de modo superficial.

Com LLM configurado, o consolidator recebe:

- observações da sessão, projetadas dentro de um orçamento;
- a versão atual da página de sessão no fluxo single-page;
- preferências do projeto;
- em alguns fluxos, snapshots de slots.

O prompt proíbe invenção, prioriza o estado final da sessão, evita tutorial ou expansão especulativa, mantém o idioma do usuário e exige structured output. O modo multi-page pode produzir memórias episódicas, semânticas, regras, decisões, gotchas e slots.

### Pipeline de geração

O caminho de captura é:

1. O hook limita o payload e aplica uma política de captura.
2. O evento vai para um spool local durável e é reenviado até confirmação ou expiração.
3. O servidor sanitiza o conteúdo e cria uma observação idempotente em SQLite.
4. `PreCompact` atualiza `sessions/<id>.md`, via LLM quando disponível ou synth determinístico como fallback.
5. `SessionEnd` cria uma página determinística para sessões substantivas, pode gerar handoff e pode enfileirar consolidação LLM.
6. O consolidator reescreve páginas canônicas e o store cria versões imutáveis ligadas por `supersedes`.

O loop de auto-improvement é separado. Ele seleciona sessões com um mínimo de observações, duração e confiança, pede ao LLM propostas com evidências, persiste essas propostas e escreve sidecars em `_pending/`. Esses sidecars não são memória canônica nem entram em busca. Quando aprovadas, as propostas alteram páginas canônicas usando snapshots e hashes para evitar sobrescrever mudanças concorrentes.

### Duplicidade, contradição e estado atual

O writer não cria uma nova versão quando o conteúdo e metadados são idênticos. Quando há mudança, mantém versão imutável anterior, marca a atual e cria relação de supersessão. Esse é um controle forte de histórico e rollback.

O prompt de consolidação manda preferir o estado final, e o lint detecta páginas vazias, títulos duplicados, staleness e links quebrados. Opcionalmente um LLM procura conflitos semânticos em uma janela limitada das páginas mais acessadas.

As garantias semânticas não são completas:

- o multi-page não recebe necessariamente o corpo completo de toda página semântica que pode substituir;
- a evidência de uma proposta precisa ter texto, mas o validator não confirma que a citação aparece na fonte indicada;
- páginas semânticas geradas em batch nem sempre recebem automaticamente a sessão de origem;
- o lint semântico observa somente uma amostra do corpus.

### Validação, revisão e privacidade

A sanitização cobre famílias conhecidas de credenciais, URLs autenticadas, PEM, JWT e paths sensíveis, além de padrões configuráveis. Ela não promete detectar toda string de alta entropia. O spool é escrito antes da sanitização server-side e pode conter payload bruto temporariamente, embora use permissões restritas.

A política de captura pode descartar buscas, listagens e paths ignorados ou reduzir eventos a metadados. Um modo allowlist pode bloquear integralmente projetos sem opt-in; no modo histórico/denylist, a ausência de marker ainda permite captura.

A revisão de auto-improvement não é necessariamente humana. `require_approval` é `false` por padrão, de modo que propostas válidas podem ser aprovadas automaticamente. A infraestrutura oferece staging, hashes e trilha de auditoria, mas o default privilegia automação.

### Qualidade provável

- **Cobertura alta:** muitos hooks e spool resiliente reduzem perdas.
- **Precisão média a alta, dependendo do modo:** synth determinístico é fiel porém raso; LLM é rico porém probabilístico.
- **Riqueza alta com consolidação LLM:** múltiplos tipos, observações, estado corrente e preferências.
- **Consistência temporal alta:** versionamento imutável, `supersedes`, slots e lint formam o tratamento mais completo de estado entre os projetos generalistas.
- **Rastreabilidade alta:** observações, sessão, versões, propostas e auditoria; ainda existem lacunas de provenance nas páginas semânticas de batch.
- **Ruído alto na camada bruta, controlado antes do canônico:** a qualidade depende da política de captura e dos gates de consolidação.
- **Dependência do modelo configurável:** existe um modo útil sem LLM, mas a memória semanticamente rica e o auto-improvement dependem fortemente do provider.

### Fontes primárias

- [Registro dos hooks](https://github.com/akitaonrails/ai-memory/blob/55fcce35d2bf3e52d933f83cb651b380fd67e81b/crates/ai-memory-cli/src/commands/render_shared.rs#L29-L49)
- [Capture policy](https://github.com/akitaonrails/ai-memory/blob/55fcce35d2bf3e52d933f83cb651b380fd67e81b/crates/ai-memory-hooks/src/capture_policy.rs#L407-L497)
- [Sanitização](https://github.com/akitaonrails/ai-memory/blob/55fcce35d2bf3e52d933f83cb651b380fd67e81b/crates/ai-memory-core/src/sanitize.rs#L14-L46)
- [Checkpoint e SessionEnd](https://github.com/akitaonrails/ai-memory/blob/55fcce35d2bf3e52d933f83cb651b380fd67e81b/crates/ai-memory-hooks/src/router.rs#L2760-L2969)
- [Synth determinístico](https://github.com/akitaonrails/ai-memory/blob/55fcce35d2bf3e52d933f83cb651b380fd67e81b/crates/ai-memory-hooks/src/synth.rs#L318-L395)
- [Consolidador single-page](https://github.com/akitaonrails/ai-memory/blob/55fcce35d2bf3e52d933f83cb651b380fd67e81b/crates/ai-memory-consolidate/src/consolidator.rs#L128-L242)
- [Consolidador multi-page](https://github.com/akitaonrails/ai-memory/blob/55fcce35d2bf3e52d933f83cb651b380fd67e81b/crates/ai-memory-consolidate/src/consolidator.rs#L423-L589)
- [Auto-improvement e aprovação](https://github.com/akitaonrails/ai-memory/blob/55fcce35d2bf3e52d933f83cb651b380fd67e81b/crates/ai-memory-consolidate/src/auto_improve_schedule.rs#L388-L490)
- [Versionamento e supersessão](https://github.com/akitaonrails/ai-memory/blob/55fcce35d2bf3e52d933f83cb651b380fd67e81b/crates/ai-memory-store/src/ops.rs#L823-L927)

## Palinode

### Gatilhos e entradas

Palinode combina quatro rotas de geração:

- `palinode_save` e a skill de sessão permitem ao coding agent gravar explicitamente decisões, insights, snapshots e pessoas.
- O endpoint `session-end` recebe resumo, decisões e blockers formulados pelo agente e faz dual-write em log diário e memória tipada individual.
- O plugin OpenClaw captura as últimas mensagens ao final de cada turno e as anexa a uma nota diária; também pode anexar um resumo determinístico a um status de projeto.
- A consolidação semanal usa um LLM para transformar notas recentes e insights em operações sobre fatos existentes.

Existe também um prompt de extração por conversa e um contrato em `PROGRAM.md`. Contudo, no plugin OpenClaw analisado, o prompt é lido mas não enviado a um LLM: o próprio código registra que o plugin não possui `api.llm` e salva um recorte da sessão para processamento posterior. Assim, não se deve atribuir ao hook atual toda a extração automática prometida pelo documento comportamental.

O prompt de “trajectory extraction” adicionado ao repositório é usado pelo adapter do benchmark LongMemEval-V2, não pelo fluxo geral de coding agents. Ele é uma evidência útil da política de geração, mas não o caminho padrão do produto.

### Quem gera e qual contexto usa

Na captura explícita e no `session-end`, o coding agent formula a memória usando o contexto da sessão e, quando segue a skill, pesquisa memórias anteriores antes de escrever.

Na consolidação, um LLM OpenAI-compatible recebe:

- fatos existentes com IDs no arquivo alvo;
- decisões do projeto que não estejam marcadas como `superseded`, tratadas como restrições;
- notas dos últimos dias, agrupadas por entidades e fallback de keywords;
- o prompt de compaction.

O contexto de notas é limitado a cerca de 6.000 caracteres, no máximo 1.500 por nota, priorizando as mais recentes. As decisões também possuem orçamento próprio. Esse recorte controla custo, mas pode omitir uma evidência relevante de uma sessão longa. Há ainda uma lacuna na seleção: o código exclui somente `status: superseded`, de modo que decisões `archived` ou `deprecated` podem entrar como se ainda fossem restrições vigentes.

### Pipeline de save e consolidação

O `save_memory` recebe conteúdo já formulado e um tipo. Ele:

1. rejeita envelopes acidentalmente enviados como conteúdo;
2. sanitiza slug e deriva categoria do tipo;
3. valida status, visibility, update policy, epistemic e confidence;
4. executa sobre o corpo `content` um security scan parcial contra padrões enumerados de prompt injection, exfiltração, JavaScript e excesso de caracteres especiais;
5. normaliza entidades, fontes, quote hashes, claims e relações `contradicts`/`backed_by`;
6. grava Markdown canônico;
7. opcionalmente cria commit e push;
8. indexa a nota em busca vetorial e FTS;
9. agenda enriquecimentos e verificação de contradição não bloqueantes.

Uma nota com slug derivado que colide com conteúdo diferente recebe sufixo em vez de sobrescrever silenciosamente. Um slug explícito representa atualização da mesma memória e pode substituir seu conteúdo.

A consolidação separa uma etapa probabilística de uma etapa determinística:

1. O LLM devolve JSON com `KEEP`, `UPDATE`, `MERGE`, `SUPERSEDE`, `ARCHIVE` ou `RETRACT`.
2. O parser recupera e filtra operações válidas.
3. `dry_run` mostra as mudanças propostas sem escrever.
4. Na aplicação, o executor atua somente no corpo Markdown, preservando frontmatter.
5. Updates preservam IDs; merges, supersessões, arquivamentos e retrações mantêm histórico.
6. Relações `backed_by` afetadas por uma retirada são marcadas para revisão.
7. Notas de origem só são arquivadas se seu grupo foi consolidado com sucesso.
8. Cada arquivo alterado pode receber commit separado.

### Duplicidade, contradição e estado atual

Palinode possui vários níveis de controle:

- A política manda pesquisar por mesmo tipo, sobreposição de entidades e similaridade semântica antes de escrever.
- Saves idênticos com slug derivado reutilizam o arquivo; colisões diferentes são desambiguadas.
- A consolidação faz merge de fatos redundantes e supersessão ou retração de fatos antigos.
- `update_policy: replace` protege documentos de estado atual contra operações que criariam um fork histórico inadequado.
- Contradições podem ser registradas por relações tipadas sem escolher automaticamente um vencedor.
- Um review offline detecta staleness, open questions, contradições, órfãos, wiki drift e backing retirado, propondo operações sem aplicá-las.
- Citações podem conter quote e hash; claims podem apontar para spans; relações `backed_by` permitem propagar perda de sustentação.

Essa é a representação epistemológica mais rica da comparação. Ao mesmo tempo, parte dela é opcional e nem toda memória possui fonte ou claim. O quote hash prova integridade entre quote e hash fornecido, não que a quote exista no arquivo referenciado nem que ela implique a conclusão.

### Validação, revisão e privacidade

O writer possui schemas e validações fortes, mas a revisão varia conforme a rota:

- `palinode_save` escreve diretamente no canônico.
- `session-end` escreve logs e memória tipada diretamente.
- consolidação pode ser pré-visualizada com `dry_run`, mas o endpoint usa `dry_run: false` por padrão;
- o review periódico é apenas consultivo e nunca aplica suas propostas.

O security scan de `save_memory` é uma proteção parcial da rota estruturada, não um scanner geral de credenciais ou PII. Ele examina somente o corpo `content`; `title`, `metadata`, sources e outros campos de frontmatter não passam por esse filtro. Há ainda caminhos de plugin que escrevem diretamente com `fs.appendFileSync`, contornando o writer, seus schemas, o scan, os claims e o commit choke point. A captura bruta de sessão traz, portanto, um risco de privacidade maior que o projeto atual.

### Qualidade provável

- **Cobertura alta:** explícito, session-end, captura diária e consolidação semanal.
- **Precisão média na captura e alta potencial após consolidação/revisão:** o LLM vê estado anterior, decisões e notas, mas recortes e rotas diretas podem introduzir erro.
- **Riqueza muito alta:** tipos, entidades, rationale, epistemic, confidence, claims, citações, backing, contradições e histórico.
- **Consistência temporal muito alta:** update, merge, supersede, archive, retract, history e propagação de backing.
- **Rastreabilidade alta:** Markdown, Git, origem, datas, relações e quote hashes; campos opcionais e writes diretos reduzem uniformidade.
- **Ruído médio:** logs brutos são separados da memória curada, mas status appends e saves diretos podem acumular conteúdo.
- **Dependência do modelo alta na consolidação e moderada no restante:** executor e validações são determinísticos.

### Fontes primárias

- [Política de extração](https://github.com/phasespace-labs/palinode/blob/42f726921dae5493821480e913d7c52cbb046c6d/PROGRAM.md#L18-L35)
- [Critérios e limites de extração](https://github.com/phasespace-labs/palinode/blob/42f726921dae5493821480e913d7c52cbb046c6d/PROGRAM.md#L103-L132)
- [Update logic](https://github.com/phasespace-labs/palinode/blob/42f726921dae5493821480e913d7c52cbb046c6d/PROGRAM.md#L453-L503)
- [Prompt de compaction](https://github.com/phasespace-labs/palinode/blob/42f726921dae5493821480e913d7c52cbb046c6d/specs/prompts/compaction.md#L3-L51)
- [`save_memory`](https://github.com/phasespace-labs/palinode/blob/42f726921dae5493821480e913d7c52cbb046c6d/palinode/core/save.py#L230-L309)
- [Validação e scan no save](https://github.com/phasespace-labs/palinode/blob/42f726921dae5493821480e913d7c52cbb046c6d/palinode/core/save.py#L442-L503)
- [Escopo do security scan](https://github.com/phasespace-labs/palinode/blob/42f726921dae5493821480e913d7c52cbb046c6d/palinode/core/store.py#L93-L151)
- [Seleção de decisões da consolidação](https://github.com/phasespace-labs/palinode/blob/42f726921dae5493821480e913d7c52cbb046c6d/palinode/consolidation/runner.py#L88-L139)
- [Proveniência, claims e escrita](https://github.com/phasespace-labs/palinode/blob/42f726921dae5493821480e913d7c52cbb046c6d/palinode/core/save.py#L688-L753)
- [Construção do contexto da consolidação](https://github.com/phasespace-labs/palinode/blob/42f726921dae5493821480e913d7c52cbb046c6d/palinode/consolidation/runner.py#L477-L581)
- [Dry-run e apply](https://github.com/phasespace-labs/palinode/blob/42f726921dae5493821480e913d7c52cbb046c6d/palinode/consolidation/runner.py#L786-L968)
- [Executor de operações](https://github.com/phasespace-labs/palinode/blob/42f726921dae5493821480e913d7c52cbb046c6d/palinode/consolidation/executor.py#L125-L204)
- [Review consultivo](https://github.com/phasespace-labs/palinode/blob/42f726921dae5493821480e913d7c52cbb046c6d/palinode/core/review.py#L1-L21)
- [Implementação atual da captura OpenClaw](https://github.com/phasespace-labs/palinode/blob/42f726921dae5493821480e913d7c52cbb046c6d/plugin/index.ts#L1157-L1246)

## claude-mem

### Gatilhos e entradas

`claude-mem` observa automaticamente prompts, tool calls e encerramentos por hooks. No worker local padrão:

- `UserPromptSubmit` inicia ou continua a sessão;
- `PostToolUse` envia nome, input e output da ferramenta;
- `Stop` extrai a última resposta textual do assistente para produzir um summary;
- `SessionStart` injeta contexto anterior.

O transcript JSONL funciona como fonte de recuperação para eventos perdidos pelo buffer em memória. O sistema persiste tanto material de sessão quanto memórias sintetizadas.

### Quem gera e qual contexto usa

Um observador LLM separado, usando Claude, Gemini ou OpenRouter, converte eventos em observações XML. No worker local ele mantém uma conversa stateful sobre a sessão. Quando essa conversa cresce demais, o sistema a recicla e inicializa a próxima com memória do projeto, por padrão até 50 observações e 10 summaries.

Cada tool call é apresentado com o que ocorreu, horário, working directory, parâmetros e resultado. O modelo gera tipo, título, subtítulo, fatos, narrativa, conceitos e arquivos lidos/modificados. O modo de código manda ignorar operações rotineiras e preservar sinal técnico durável.

Payloads grandes podem passar por uma chamada LLM extra de condensação antes da observação final. Se ela falha, há truncamento head/tail. Essa etapa preserva mais informação que corte simples, mas pode introduzir uma primeira perda ou distorção probabilística.

O summary de `Stop` contém request, investigated, learned, completed, next steps e notes. A entrada explícita é somente a última resposta do assistente, embora o observador local também possua seu histórico stateful.

### Pipeline e destino

No worker local:

1. Hooks normalizam eventos de diferentes agentes.
2. Filtros de projeto/ferramenta e regras de privacidade removem eventos ou tags privadas.
3. Tool uses são registrados e eventos entram num buffer não durável.
4. O provider LLM devolve XML.
5. Um parser textual extrai observações e summaries.
6. O sistema recalcula deterministicamente arquivos modificados a partir do input real da ferramenta.
7. Observações e summary são gravados numa transação SQLite.
8. FTS5 e Chroma indexam narrativa, fatos, conceitos e arquivos para recuperação progressiva.

Existe um runtime server alternativo com Postgres, outbox transacional e BullMQ. Ele melhora idempotência e liga cada observação ao evento e job de origem, mas gera cada item single-shot, sem o histórico conversacional do worker. No commit analisado, o job de summary usa um prompt que solicita `<observation>` e depois procura `<summary>`, podendo concluir com zero summaries. Além disso, o hook padrão de `SessionStart` continua consultando o worker local, não as memórias do server.

### Duplicidade, contradição e estado atual

Há dedupe de prompts próximos, tool uses identificados e observações por hash. O hash local usa apenas sessão, título e narrativa. Isso evita repetições, mas pode colapsar duas observações com fatos ou arquivos diferentes; observações apenas com fatos e sem título/narrativa também podem compartilhar identidade.

Não foi encontrado um mecanismo semântico de supersessão, invalidação, contradição ou validade temporal. As observações são essencialmente append-only. Um fato antigo continua pesquisável depois que um novo fato o contradiz.

Esse é o principal limite de qualidade de longo prazo do `claude-mem`: ele possui excelente cobertura episódica, mas não mantém por si só uma representação confiável do estado atual.

### Validação, revisão e privacidade

O parser XML rejeita respostas vazias e blocos sem conteúdo, mas é permissivo:

- tipo inválido é logado e ainda pode ser preservado;
- conceitos não são validados estritamente;
- prosa ao redor do XML é aceita;
- o parser não confirma se a chamada esperava observation ou summary;
- não existem confidence, segunda passagem factual ou revisão humana obrigatória.

No worker local, prompts, inputs, outputs e última resposta são limpos antes de entrar na memória, e tags privadas ou de contexto são removidas. O observador Claude roda sem tools e sem MCP herdado. Isso limita ações do observador, mas não impede que conteúdo de ferramenta tente envenenar semanticamente a memória.

No runtime server, eventos brutos são persistidos antes da remoção de tags privadas usada na geração. Portanto “private” impede principalmente envio ao LLM e transformação, não toda persistência do dado bruto.

### Qualidade provável

- **Cobertura muito alta:** hooks e recuperação por transcript observam quase todo o trabalho.
- **Precisão média:** inputs reais e arquivos recalculados ajudam, mas o parser e a falta de verificação factual deixam o LLM decidir o conteúdo.
- **Riqueza alta:** fatos, narrativa, conceitos, arquivos e summaries.
- **Consistência temporal baixa:** append-only, sem supersessão ou contradição.
- **Rastreabilidade média no worker e alta no server:** o server liga jobs e eventos, mas possui lacunas funcionais no commit analisado.
- **Ruído médio a alto:** o prompt tenta filtrar rotina, mas cada tool call pode alimentar o gerador.
- **Dependência do modelo muito alta:** seleção, compressão, classificação e narrativa mudam conforme provider e modelo.

### Fontes primárias

- [Configuração de hooks](https://github.com/thedotmack/claude-mem/blob/8bc631a71a487424b866756e43a6efa4574cc66b/plugin/hooks/hooks.json#L17-L84)
- [Ingestão local de observações](https://github.com/thedotmack/claude-mem/blob/8bc631a71a487424b866756e43a6efa4574cc66b/src/services/worker/http/shared.ts#L65-L185)
- [Contrato do buffer não durável](https://github.com/thedotmack/claude-mem/blob/8bc631a71a487424b866756e43a6efa4574cc66b/src/services/worker/SessionMessageBuffer.ts#L21-L40)
- [Prompt/schema de observação](https://github.com/thedotmack/claude-mem/blob/8bc631a71a487424b866756e43a6efa4574cc66b/src/sdk/prompts.ts#L24-L61)
- [Prompt/schema de summary](https://github.com/thedotmack/claude-mem/blob/8bc631a71a487424b866756e43a6efa4574cc66b/src/sdk/prompts.ts#L192-L223)
- [Parser XML](https://github.com/thedotmack/claude-mem/blob/8bc631a71a487424b866756e43a6efa4574cc66b/src/sdk/parser.ts#L41-L85)
- [Verificação determinística de arquivos](https://github.com/thedotmack/claude-mem/blob/8bc631a71a487424b866756e43a6efa4574cc66b/src/services/worker/agents/ResponseProcessor.ts#L39-L77)
- [Persistência transacional local](https://github.com/thedotmack/claude-mem/blob/8bc631a71a487424b866756e43a6efa4574cc66b/src/services/sqlite/SessionStore.ts#L2686-L2801)
- [Dedupe por hash](https://github.com/thedotmack/claude-mem/blob/8bc631a71a487424b866756e43a6efa4574cc66b/src/services/sqlite/observations/store.ts#L5-L13)

## claude-bedrock

### Gatilhos e entradas

`claude-bedrock` é um plugin declarativo para um vault Obsidian. Não possui um runtime próprio que imponha os algoritmos; suas skills orientam o Claude a usar ferramentas externas e editar o vault.

Há três rotas principais:

- `/bedrock:preserve` recebe texto, listas, notas de reunião ou contexto da sessão.
- `/bedrock:learn` importa Confluence, Google Docs, GitHub, URLs, arquivos ou diretórios e delega extração de grafo à skill externa Graphify.
- `/bedrock:compress` revisa o vault e propõe conceitos recorrentes, aliases, recategorização e consolidação de entidades.

O único hook de fim de sessão encontrado é voltado ao relatório de erros do plugin, não à captura automática de memória.

### Quem gera e qual contexto usa

O Claude que executa as skills formula a memória. Em preserve direto ele lê definições de entidades, enumera o vault e compara arquivo, nome, aliases e IDs Graphify. Pode enriquecer atores com GitHub e Atlassian em modo best-effort.

No learn, o contexto pode incluir documento convertido, `graph.json`, comunidades, god nodes, confiança de arestas, relações semânticas e conteúdo existente. Parte central dessa extração pertence ao Graphify, que não está implementado neste repositório; Bedrock apenas define como instalar e consumir a skill externa.

### Pipeline, dedupe e consolidação

`preserve` identifica entidades, classifica cada uma em categorias Zettelkasten, procura correspondências e apresenta uma proposta ao usuário. O default é confirmar com `yes`, `no` ou `adjust` antes de escrever.

O matching usa nome de arquivo, `name`/`title`, aliases, forma sem hífens e IDs Graphify. Ambiguidades são levadas à proposta. Fontes por URL são deduplicadas.

As regras de atualização variam por tipo:

- actors podem substituir estado obsoleto;
- people, teams, concepts e topics tendem a append-only;
- wikilinks e sources são preservados.

`compress` pode criar conceitos após recorrência, promover fleeting notes, recategorizar entidades e consolidar duas entidades mantendo uma indicação na secundária. Não existe um modelo formal de contradições ou counter-evidence. A combinação de append-only e ausência de resolução estruturada pode conservar fatos antigos como igualmente atuais.

### Validação, revisão e privacidade

Os schemas são templates e critérios em linguagem natural, não validators executáveis uniformes. A qualidade depende da aderência do Claude às instruções.

A confirmação humana é uma força importante do fluxo padrão, mas há exceções. `compress --mode cron` pode pular confirmação para reparos; o merge de artefatos Graphify ocorre antes da confirmação das entidades e já pode alterar o vault.

Há instruções para não persistir credenciais, tokens, PANs ou CVVs, mas não foi encontrado scanner obrigatório no caminho geral de escrita. Paths locais também podem ser persistidos como source URL.

### Qualidade provável

- **Cobertura média manual e alta em ingestão de documentos:** não há captura automática de sessão.
- **Precisão média:** revisão humana e matching ajudam, mas schemas e transformação são guiados por prompt.
- **Riqueza muito alta para conhecimento organizacional:** entidades, relações, documentos e contexto externo.
- **Consistência temporal média:** lifecycle de topics/fleeting existe, mas muitas categorias são append-only e contradições não são estruturadas.
- **Rastreabilidade média a alta:** fontes e Git ajudam; Graphify externo e critérios declarativos dificultam auditar todas as transformações.
- **Ruído variável:** ingestões grandes e append-only podem acumular conteúdo; compress procura reduzir isso.
- **Dependência do modelo muito alta:** o Claude é parser, classificador, deduplicador e editor.

### Fontes primárias

- [`preserve`: entradas e análise](https://github.com/iurykrieger/claude-bedrock/blob/7cac16ceb6064b7fbb4ec45c3ad4be31b9d9c9f0/skills/preserve/SKILL.md#L310-L377)
- [`preserve`: matching](https://github.com/iurykrieger/claude-bedrock/blob/7cac16ceb6064b7fbb4ec45c3ad4be31b9d9c9f0/skills/preserve/SKILL.md#L507-L578)
- [`preserve`: confirmação](https://github.com/iurykrieger/claude-bedrock/blob/7cac16ceb6064b7fbb4ec45c3ad4be31b9d9c9f0/skills/preserve/SKILL.md#L582-L625)
- [`preserve`: atualização](https://github.com/iurykrieger/claude-bedrock/blob/7cac16ceb6064b7fbb4ec45c3ad4be31b9d9c9f0/skills/preserve/SKILL.md#L700-L736)
- [`learn`: pipeline Graphify](https://github.com/iurykrieger/claude-bedrock/blob/7cac16ceb6064b7fbb4ec45c3ad4be31b9d9c9f0/skills/learn/SKILL.md#L337-L387)
- [`compress`: operações](https://github.com/iurykrieger/claude-bedrock/blob/7cac16ceb6064b7fbb4ec45c3ad4be31b9d9c9f0/skills/compress/SKILL.md#L634-L775)
- [Hook de erro, não captura de memória](https://github.com/iurykrieger/claude-bedrock/blob/7cac16ceb6064b7fbb4ec45c3ad4be31b9d9c9f0/hooks/hooks.json#L1-L15)

## agent-memory

### Gatilhos e entradas

`agent-memory` instrui o coding agent a atualizar memória continuamente ao concluir decisões, descobrir pitfalls, alterar fatos de módulo, registrar convenções, finalizar uma sessão ou arquivar conhecimento obsoleto. Há também um bootstrap explícito que manda o agente ler build files, CI, entry points e módulos antes de propor memória inicial.

A unidade de entrada é uma atualização estruturada com:

- `intent`;
- `rationale`;
- uma ou mais operações;
- `sources`;
- `grounding` opcional;
- `confidence`;
- `owner`.

### Quem gera e qual contexto usa

O coding agent formula todo o conteúdo. O servidor não chama LLM. Diferentemente do `/remember` atual, o agente é instruído a explorar código, testes e documentação durante o bootstrap e usa o contexto normal da tarefa para redigir a atualização.

No início de uma tarefa, o agente recebe estado corrente de branch, estado compartilhado, convenções e índice. A busca usa FTS5/BM25, escopo, branch, arquivos modificados e stores federados, sempre incluindo estado corrente. Seções quase idênticas são deduplicadas no context pack para economizar budget.

### Pipeline e destino

O servidor aceita intents como `update_current`, `session_log`, `add_pitfall`, `record_decision`, `refresh_module`, `update_conventions` e `archive_stale`. As operações permitem criar, append, substituir seção ou corpo, renomear e arquivar.

O pipeline:

1. valida intent e operações;
2. resolve paths e categorias;
3. obtém lock;
4. lê baseline;
5. aplica operações sequencialmente em memória;
6. valida Markdown e schema das seções afetadas;
7. executa scan de secrets/PII;
8. valida provenance e grounding;
9. decide entre `apply`, `stage` ou `server_only`;
10. grava atomicamente e reindexa, ou cria staging com request, checksums e post-state completo.

Memórias correntes, logs de sessão e alguns pitfalls podem ser aplicados diretamente. Decisions, modules, conventions e operações destrutivas normalmente passam por staging. A política depende do intent informado.

O staging vive em `.agent-memory/staging/<id>/`, não é versionado e é removido após apply. A memória durável fica em `conventions.md`, `decisions.md`, `pitfalls.md`, `modules/` e `archive/`; current e sessions são locais por padrão. Git auto-stage é desabilitado por padrão, então “aplicado” não significa necessariamente commitado.

### Duplicidade, contradição e estado atual

A deduplicação por Jaccard ocorre na recuperação, não na escrita. Não há detector semântico que impeça duas memórias equivalentes.

O agente pode substituir seções, atualizar corpo e arquivar conteúdo; decisões possuem estados como active, superseded, deprecated e proposed. O merge driver resolve concorrência estrutural por seção. Nenhum desses mecanismos detecta sozinho que duas afirmações se contradizem.

Portanto, a consistência de estado atual é melhor que no projeto atual porque as operações já podem atualizar o canônico, mas a escolha semântica continua nas mãos do LLM e do revisor.

### Validação, evidência, privacidade e revisão

Os schemas por categoria, operações byte-preserving, locks, hashes e detecção de drift oferecem boa integridade estrutural. Decisions podem restringir tipos de fonte e confidence é enumerado. Grounding pode ligar a proposta a um context pack por digest e nonce single-use.

Entretanto, `Source.Ref` não é verificado quanto a existência ou conteúdo, e grounding não é obrigatório no schema padrão. A provenance estruturada fica no `proposal.json` de staging, mas não é injetada automaticamente no Markdown; como o staging é apagado no apply, a rastreabilidade pós-apply depende do conteúdo redigido e de Git.

O scanner é mais amplo que o do projeto atual em alguns padrões, incluindo alta entropia e cartões, mas pode não examinar metadados como rationale, source refs e owner antes de serializar a requisição no staging.

Nem toda memória recebe revisão. Algumas categorias aplicam diretamente, e o routing usa o intent declarado. Isso aumenta fluidez e cobertura, mas permite uma atualização errada atingir o canônico mais cedo que no `git-push-memories`.

### Qualidade provável

- **Cobertura alta se o agente obedecer à skill:** múltiplos gatilhos durante toda a sessão.
- **Precisão média a alta:** schemas, staging e drift ajudam; fontes não são verificadas e direct apply reduz o gate.
- **Riqueza alta:** intents, rationale, confidence, sources, branch state, sessões e operações estruturadas.
- **Consistência temporal média a alta:** replace e archive existem, mas contradição e obsolescência não são automáticas.
- **Rastreabilidade alta antes do apply e média depois:** staging rico é descartado.
- **Ruído médio:** mais categorias e atualizações proativas que o projeto atual, algumas sem revisão.
- **Dependência do modelo alta na semântica e baixa na infraestrutura:** qualquer coding agent pode gerar, mas a qualidade varia por modelo.

### Fontes primárias

- [Skill do Claude](https://github.com/xChuCx/agent-memory/blob/e42f455865538a59110c6510ae8e340969feb810/internal/adapters/claude/SKILL.md#L23-L69)
- [Schema MCP da proposta](https://github.com/xChuCx/agent-memory/blob/e42f455865538a59110c6510ae8e340969feb810/internal/mcp/propose.go#L17-L52)
- [Intents e routing](https://github.com/xChuCx/agent-memory/blob/e42f455865538a59110c6510ae8e340969feb810/internal/memory/routing.go#L18-L50)
- [Pipeline de update](https://github.com/xChuCx/agent-memory/blob/e42f455865538a59110c6510ae8e340969feb810/internal/memory/update.go#L175-L325)
- [Validação e decisão apply/stage](https://github.com/xChuCx/agent-memory/blob/e42f455865538a59110c6510ae8e340969feb810/internal/memory/update.go#L327-L618)
- [Context pack](https://github.com/xChuCx/agent-memory/blob/e42f455865538a59110c6510ae8e340969feb810/internal/memory/fetch.go#L289-L525)
- [Provenance](https://github.com/xChuCx/agent-memory/blob/e42f455865538a59110c6510ae8e340969feb810/internal/memory/provenance.go#L42-L165)
- [Staging e drift](https://github.com/xChuCx/agent-memory/blob/e42f455865538a59110c6510ae8e340969feb810/internal/memory/staging.go#L215-L408)

## twiceshy

### Gatilhos e entradas

`twiceshy` não tenta guardar todo o domínio de um projeto. Ele gera experience records especializados em traps, fixes, dead ends, conventions e workflows.

As entradas podem vir de:

- `record_experience`, chamado explicitamente por um agente;
- `twiceshy learned`, preenchido por flags;
- importadores determinísticos de OSV, deprecations e EOL;
- retrospectiva de sessão no hook `SessionEnd`;
- minerador de commits externos;
- outcome reports, que criam counter-memory sem alterar o registro contestado.

### Quem gera e qual contexto usa

Na retrospectiva, um LLM recebe a cauda limitada do transcript e é instruído a extrair apenas lições claras, novas, generalizáveis e não específicas do projeto, no máximo oito por padrão. O transcript é cercado como DATA, delimitadores falsos são neutralizados e controles são removidos.

O schema pedido contém kind, título, sintoma, assinaturas de erro, ecossistema, pacote, causa raiz, fix e narrativa. Importadores, por outro lado, transformam fontes conhecidas de modo determinístico e não dependem de uma interpretação livre de sessão.

### Pipeline e promoção

Todo draft passa por um pipeline comum:

1. O índice avalia cada assinatura, depois o resumo ou título.
2. Um match exato `known` cancela a criação.
3. Um match `similar` mantém candidatos para revisão; `novel` segue normalmente.
4. O sistema constrói um record com status `quarantined` e provenance.
5. Um safety gate procura secrets, harmful code e PII.
6. Consistency checks determinísticos procuram defeitos de transcrição.
7. JSON Schema e regras cross-field validam o record.
8. O arquivo é persistido atomicamente em `experience/YYYY/NNNN-slug.md`.

Um quarantined record não entra no canal confiável. Para promoção de uma memória executável, o sistema pode exigir:

- causa raiz substantiva;
- repro positivo;
- execução fail-before/pass-after em sandbox gVisor;
- holding attestation;
- aprovação de um judge;
- validação final do record.

Registros advisory ou de prosa sem repro usam painéis de modelos de famílias diferentes. Falha, output malformado ou indisponibilidade deixam o registro em quarentena. Em alguns fluxos existe PR e janela de veto humano, mas aprovação humana não é obrigatória para toda promoção.

### Duplicidade, contradição e temporalidade

Fingerprint exato impede duplicata conhecida; BM25 identifica similares. O batch também deduplica por assinatura ou título. Similaridade não bloqueia automaticamente a criação porque duas experiências próximas podem ser distintas.

Counter-memory é aditiva: um outcome report cria um novo `dead-end` com `provenance.disputes`. Se uma falha contestando uma memória validada é reproduzida e aprovada, a memória pode ser rebaixada a `stale`; múltiplos relatos independentes podem levá-la a `disputed`.

O schema representa dois eixos temporais:

- `valid.from` e `valid.until`;
- applicability por versões em `applies_to`.

Também existe `superseded_by`. Não há delete: registros permanecem como trilha histórica.

### Validação, evidência e privacidade

Esse é o mecanismo mais forte da comparação para afirmações técnicas executáveis. A prova não é apenas um path ou uma citação; o repro pode ser executado com disciplina fail-to-pass, limites de CPU, memória, processos, timeout, rootfs read-only, sem network e imagem fixada por digest.

O judge verifica meaning, scope, usefulness, license e poison. Ainda existem limites: advisory imports podem confiar que o importador estabeleceu a fonte sem navegar no URL, e registros de prosa não possuem a mesma garantia de uma reprodução executável.

O scanner cobre todos os campos textuais do record antes da escrita normal. A retrospectiva examina secrets localmente e no servidor. Transcripts podem, porém, ser enviados a provider externo, e o minerador externo pode gravar payload bruto na fila antes do screen final.

### Qualidade provável

- **Cobertura alta dentro de experiências técnicas e baixa fora desse domínio:** não é memória geral de arquitetura, pessoas ou produto.
- **Precisão muito alta para records com repro validado:** nenhum outro projeto exige prova executável comparável.
- **Precisão média a alta para advisory/prosa:** depende de importadores e painéis de modelo.
- **Riqueza alta e especializada:** sintoma, signatures, causa, fix, dead ends, guard, applicability e provenance.
- **Consistência temporal muito alta:** quarantine, validated, stale, disputed, superseded e counter-evidence.
- **Rastreabilidade muito alta:** fonte, sessão, licença, prova, judge, promoção e demotion.
- **Ruído baixo no canal validado:** a quarentena absorve candidatos duvidosos.
- **Dependência do modelo baixa para registros executáveis após promoção e alta na primeira extração de transcript:** prova e validators reduzem a confiança necessária no gerador.

### Fontes primárias

- [Schema do experience record](https://github.com/dotts-h/twiceshy/blob/aa8f1d88b23ce26b9add249ee2f507d350aca12c/docs/SCHEMA.md#L20-L49)
- [Provenance e evidência](https://github.com/dotts-h/twiceshy/blob/aa8f1d88b23ce26b9add249ee2f507d350aca12c/docs/SCHEMA.md#L101-L119)
- [Lifecycle](https://github.com/dotts-h/twiceshy/blob/aa8f1d88b23ce26b9add249ee2f507d350aca12c/docs/SCHEMA.md#L121-L146)
- [Prompt da retrospectiva](https://github.com/dotts-h/twiceshy/blob/aa8f1d88b23ce26b9add249ee2f507d350aca12c/internal/retro/analyzer.go#L69-L121)
- [Dedupe, quarentena e safety gate](https://github.com/dotts-h/twiceshy/blob/aa8f1d88b23ce26b9add249ee2f507d350aca12c/internal/ingest/prepare.go#L64-L159)
- [Eligibility para promoção](https://github.com/dotts-h/twiceshy/blob/aa8f1d88b23ce26b9add249ee2f507d350aca12c/internal/promote/promote.go#L140-L207)
- [Attestation e judge](https://github.com/dotts-h/twiceshy/blob/aa8f1d88b23ce26b9add249ee2f507d350aca12c/internal/promote/promote.go#L210-L284)

## Comparação transversal

### Cobertura de captura

`claude-mem` e `ai-memory` ficam no extremo oposto: observam eventos ao longo de toda a sessão. `Palinode` e `Basic Memory` combinam hooks mais seletivos com captura deliberada. `agent-memory` depende de o coding agent obedecer a gatilhos descritos em skill. `twiceshy` usa hooks e importadores, mas filtra para uma classe específica de experiência.

### Qualidade da formulação inicial

No projeto atual, Basic Memory, agent-memory e boa parte do Palinode, a formulação é feita pelo próprio coding agent que realizou o trabalho. Isso oferece acesso ao raciocínio e contexto corrente, mas também acopla a memória à qualidade e à disciplina desse modelo.

`claude-mem`, `ai-memory`, a consolidação Palinode e a retrospectiva twiceshy usam um modelo separado. Isso cria uma segunda interpretação do ocorrido: pode filtrar melhor o sinal, mas também perder nuance ou inventar conexões.

Mecanismos determinísticos, como o checkpoint automático do Basic Memory e o synth de sessão do ai-memory, têm menor risco de alucinação e menor riqueza. Os dois atributos se movem juntos: retirar o LLM reduz interpretação errada, mas também reduz capacidade de extrair causa, decisão e implicação.

### Uso do repositório de código

Nenhum servidor analisado “descobre a verdade do código” automaticamente em toda geração.

- Basic Memory e agent-memory podem pedir ao coding agent Git, testes e arquivos, mas o writer confia no resultado.
- ai-memory e claude-mem capturam tool calls e paths, o que fornece evidência observacional mais forte, porém não prova toda conclusão semântica.
- Palinode aceita citações, claims e hashes, mas eles são opcionais e nem sempre verificam entailment.
- twiceshy é a exceção mais forte quando consegue executar um repro; ainda assim isso cobre uma experiência técnica específica, não todo fato de domínio.

### Duplicidade e contradição

Há três níveis de maturidade:

- **Curadoria semântica guiada pelo agente:** Basic Memory e claude-bedrock.
- **Operações e histórico, sem prova semântica completa:** agent-memory, ai-memory e Palinode.
- **Counter-evidence e revalidação executável:** twiceshy, restrito a experience records.

`claude-mem` tem bom dedupe de eventos, mas praticamente não consolida o significado. Isso evita duplicar tool calls sem impedir que fatos antigos e novos permaneçam simultaneamente autoritativos.

### Proposta versus memória canônica

Essa distinção muda a leitura de qualidade:

- Basic Memory, claude-mem e vários caminhos de Palinode escrevem diretamente no canônico.
- agent-memory mistura direct apply e staging conforme intent.
- ai-memory mistura synth direto, consolidação direta e propostas de auto-improvement; estas podem ser autoaprovadas por default.
- claude-bedrock normalmente apresenta proposta antes de editar, mas possui exceções e etapas prévias que já escrevem.
- twiceshy persiste candidatos em quarentena e só serve como validado após promoção.

Quanto maior a separação entre candidato e canônico, menor o risco de poluir a memória compartilhada, mas maior a chance de candidatos envelhecerem sem nunca produzir valor.

### Privacidade

Sistemas com hooks possuem uma superfície inevitavelmente maior. ai-memory usa sanitização e permissões fortes, mas spoola antes da sanitização server-side. claude-mem local limpa antes da memória, enquanto o runtime server persiste eventos brutos primeiro. Palinode e Basic Memory têm caminhos em que conteúdo de sessão ou nota não passa por um scanner geral. twiceshy possui scan amplo, mas pode enviar retrospectivas a providers externos e alguns importadores enfileiram bruto.

### Dependência do modelo

claude-bedrock depende quase integralmente do modelo porque sua implementação principal são skills. claude-mem depende do modelo tanto para seleção quanto para representação. ai-memory e Palinode amortecem essa dependência com synth, operações, schemas, histórico e validators. twiceshy a reduz ainda mais depois da extração quando existe prova executável.
