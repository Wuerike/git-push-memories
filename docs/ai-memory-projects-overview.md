# Panorama de projetos de memória para IA

**Data da pesquisa: 2 de setembro de 2026.**

Este documento apresenta projetos relacionados a memória para agentes de IA e compara sua aderência ao contexto do `ai-git-mem`. A pesquisa considerou, principalmente, os seguintes aspectos:

1. memória de **domínio de engenharia**, não apenas memória pessoal ou de conversa;
2. Markdown e Git como fonte canônica;
3. compartilhamento entre pessoas e repositórios;
4. captura automática e explícita;
5. consolidação orientada ao **estado atual**;
6. privacidade sem persistência das sessões;
7. publicação governada por propostas e pull requests.

Não foi encontrado um projeto que reúna simultaneamente todos esses aspectos. Os projetos mais maduros normalmente capturam conversas e usam bancos de dados ou serviços próprios. Os projetos mais próximos do modelo Git, Markdown e revisão ainda tendem a ser menores ou recentes. ([github.com](https://github.com/basicmachines-co/basic-memory))

## Critérios utilizados

Foram utilizadas duas avaliações independentes:

- **Aderência - 1 a 5:** quanto o projeto se aproxima do problema do `ai-git-mem`.
- **Maturidade - A a C:** quão confiável ele parece como referência de implementação.
  - **A:** adoção e atividade consolidadas.
  - **B:** implementação substancial e ativa, mas mais recente, menor ou ainda em evolução.
  - **C:** projeto pequeno, template, preview ou referência conceitual.

Os números de issues e PRs abaixo representam itens **abertos no momento da pesquisa**, não o total histórico. Um número alto de PRs pode significar comunidade ativa, mas também backlog de manutenção. Commits e estrelas também não são comparáveis isoladamente.

O GitHub não expôs contagens de contribuidores de forma uniforme em todos os repositórios. Em vez de adotar uma medida artificialmente homogênea, foram considerados commits, forks, fluxo de issues e PRs, releases, testes, documentação de contribuição, governança, segurança e suporte institucional.

---

## Projetos inicialmente considerados

| Projeto | Aderência | Maturidade | Avaliação |
|---|---:|---:|---|
| **Palinode** | **5/5** | **C+** | É um dos projetos conceitualmente mais próximos: Markdown versionado em Git, estados epistêmicos como `fact`, `inference` e `open_question`, evidências tipadas, busca híbrida, consolidação por operações estruturadas e validação antes da aplicação. Tem 145 commits, 33 forks, 19 issues e 1 PR aberto; a versão 0.15.0 foi atualizada em 1º de setembro de 2026. O desenho é consistente, mas a comunidade e a base de mantenedores ainda parecem pequenas. ([github.com](https://github.com/phasespace-labs/palinode)) |
| **Basic Memory** | **5/5** | **A-** | É uma referência madura para o padrão "Markdown canônico, SQLite derivado". Possui aproximadamente 3,8 mil estrelas, 268 forks, 1.928 commits, 70 issues e 6 PRs abertos, além de testes, benchmarks, documentação e integrações. Cada arquivo representa uma entidade com observações e relações. O plugin do Claude Code implementa briefing no início da sessão, checkpoint antes de compactação, captura opt-in e comandos explícitos como `/remember`. Git e PR não são o principal modelo de publicação. A licença é AGPL-3.0. ([github.com](https://github.com/basicmachines-co/basic-memory)) |
| **AI Memory Vault** | **3/5** | **C** | É principalmente um conjunto de templates, scripts e práticas para transformar um vault do Obsidian em memória para Claude Code. Tem aproximadamente 607 estrelas, 150 forks e 57 commits, sem issues ou PRs abertos. É útil para estudar organização de arquivos, notas-mestre, índices e checkpoints, mas não constitui um motor de captura, validação ou colaboração. A licença CC BY-SA 4.0 exige atenção antes da reutilização direta de conteúdo ou templates. ([github.com](https://github.com/jaredrhod/ai-memory-vault)) |
| **EverOS** | **4/5** | **B-** | Usa Markdown legível como representação canônica, com SQLite e LanceDB como índices locais, file watcher e uma etapa de reflexão que agrupa e refina memórias. Tem aproximadamente 12,6 mil estrelas, 913 forks, 49 issues e 29 PRs, mas somente 92 commits; a versão 1.2.1 saiu em 29 de julho de 2026. A atenção recebida é grande, mas o projeto ainda é jovem. Armazena conversas e trajetórias de agentes. ([github.com](https://github.com/EverMind-AI/EverOS)) |
| **claude-mem** | **4,5/5** | **A** | É uma referência forte para captura automática via hooks. Tem aproximadamente 93 mil estrelas, 8,2 mil forks, 2.518 commits, 38 issues e 259 PRs abertos. Usa eventos como `SessionStart`, `UserPromptSubmit`, `PostToolUse`, `Stop` e `SessionEnd`; persiste sessões, observações e resumos em SQLite e usa Chroma para busca. Sua recuperação ocorre em camadas: busca, timeline e observações completas. ([github.com](https://github.com/thedotmack/claude-mem)) |
| **MemPalace** | **3,5/5** | **A** | Tem aproximadamente 58,8 mil estrelas, 7,5 mil forks, 1.947 commits, 357 issues e 393 PRs abertos. Apresenta uma hierarquia navegável de "alas, salas e gavetas", busca temporal e autosave por hooks, inclusive antes de compactações. Também publica resultados próprios de benchmark. Seu modelo de armazenamento inclui conversas verbatim. ([github.com](https://github.com/MemPalace/mempalace)) |
| **Mem0** | **3,5/5** | **A** | É uma das principais referências gerais da categoria, com aproximadamente 64,6 mil estrelas, 7,6 mil forks, 2.621 commits, 285 issues e 435 PRs abertos. Oferece extração de fatos, memória de agentes, busca semântica, lexical e por entidades, além de raciocínio temporal. O algoritmo descrito pelo projeto em 2026 enfatiza uma extração de uma passagem com operações `ADD`, evitando `UPDATE` e `DELETE`. ([github.com](https://github.com/mem0ai/mem0)) |
| **MemoryBear** | **3/5** | **B+** | É uma plataforma substancial: aproximadamente 5,4 mil estrelas, 416 forks e 6.264 commits. Trabalha com triplas estruturadas, ontologias e campos temporais como validade e invalidação. Depende de FastAPI, Celery, PostgreSQL, Neo4j, Elasticsearch e Redis. ([github.com](https://github.com/SuanmoSuanyangTechnology/MemoryBear)) |
| **akitaonrails/ai-memory** | **5/5** | **B+** | É um dos comparadores end-to-end mais próximos. Tem aproximadamente 5,5 mil estrelas, 378 forks e 1.555 commits. Captura trabalho por hooks, aplica uma fronteira de privacidade tipada antes da persistência e consolida a sessão em páginas Markdown legíveis. O wiki é a fonte canônica sob Git e o SQLite é derivado. Também implementa relações como `causes`, `fixes` e `contradicts`, campos temporais e handoff entre diferentes agentes. Não adota um fluxo PR-first e usa aprovação automática como caminho padrão em algumas operações. ([github.com](https://github.com/akitaonrails/ai-memory)) |
| **MemMachine** | **2,5/5** | **B+** | Tem aproximadamente 3,2 mil estrelas, 208 forks, 939 commits, 66 issues e 69 PRs abertos, além de documentação de contribuição e governança. Divide a memória em episódica baseada em grafo, perfil persistido em SQL e working memory. Pressupõe um serviço de memória e bancos de dados, não arquivos Git e Markdown. ([github.com](https://github.com/MemMachine/MemMachine)) |

### Leitura comparativa

Os projetos com maior aderência ao objeto da pesquisa são:

1. **Basic Memory**, pela combinação de fonte Markdown e índice derivado.
2. **akitaonrails/ai-memory**, pelo fluxo completo integrado a coding agents.
3. **Palinode**, pelo tratamento de evidência, auditabilidade e consolidação.
4. **claude-mem**, pelo uso de hooks e recuperação progressiva.
5. **Mem0**, pela relevância na categoria e pelo modelo de memória factual acumulativa.
6. **EverOS**, pelo uso de watcher e reflexão sobre arquivos.

`MemPalace`, `MemoryBear` e `MemMachine` apresentam referências arquiteturais específicas, embora seus modelos de armazenamento sejam mais distantes dos critérios analisados. `AI Memory Vault` funciona principalmente como referência de organização e experiência de uso.

---

## Outros projetos relevantes

### Projetos substanciais

| Projeto | Aderência | Maturidade | Avaliação |
|---|---:|---:|---|
| **Graphiti** | **4,5/5** | **A** | Aproximadamente 30,5 mil estrelas, 3,1 mil forks, 962 commits, 278 issues e 202 PRs. Sua principal contribuição é o modelo temporal: fatos têm intervalos de validade, proveniência até o episódio de origem e podem ser invalidados sem desaparecer. Permite representar contradições, mudanças de verdade e consultas históricas. ([github.com](https://github.com/getzep/graphiti)) |
| **OpenViking** | **4/5** | **B+** | Aproximadamente 35,1 mil estrelas, 2,7 mil forks, 2.214 commits, 202 issues e 398 PRs. Organiza contexto como um filesystem virtual e oferece três níveis: resumo, visão geral e detalhes. A trajetória de recuperação é observável. Apesar da adoção, o próprio projeto ainda se declara em estágio inicial. Licença AGPL-3.0. ([github.com](https://github.com/volcengine/OpenViking)) |
| **zilliztech/memsearch** | **5/5** | **B** | Aproximadamente 2,6 mil estrelas, 239 forks, 455 commits, 30 issues e 209 PRs. Usa Markdown como fonte e Milvus como índice sombra, com captura automática, injeção antes das tarefas, deduplicação, watcher e recuperação em três etapas. Integra-se a vários coding agents. Seu modelo persiste conversas e depende de Milvus. ([github.com](https://github.com/zilliztech/memsearch)) |
| **Letta Code** | **4/5** | **B+** | Aproximadamente 3,2 mil estrelas, 388 forks e 3.312 commits. O MemFS registra contexto e skills em Git e pode sincronizar com um repositório GitHub escolhido pelo usuário. O escopo do projeto é um ambiente de agente stateful mais amplo, não uma base de conhecimento de domínio. ([github.com](https://github.com/letta-ai/letta-code)) |
| **LangMem** | **4/5** | **B** | Aproximadamente 1,6 mil estrelas, 186 forks e 149 commits. Separa a memória criada no "hot path" pelo agente de um gerenciador em segundo plano que extrai, consolida e atualiza conhecimento. Não acopla a solução a um armazenamento específico. ([github.com](https://github.com/langchain-ai/langmem)) |
| **Cognee** | **3,5/5** | **A** | Aproximadamente 30,4 mil estrelas, 3 mil forks e 9.792 commits. Possui pipelines modulares, grafo, vetores, ontologias, operações de lembrar, recuperar, esquecer e melhorar, além de plugin para Claude Code. Captura prompts, tool traces e respostas e opera como uma plataforma de dados mais ampla. ([github.com](https://github.com/topoteretes/cognee)) |
| **Supermemory** | **3,5/5** | **A** | Aproximadamente 29,2 mil estrelas, 2,6 mil forks e 1.902 commits. Trabalha com alterações temporais, contradições, esquecimento automático, perfis e escopos de projeto; também possui integrações com Claude Code e outros agentes. A memória compartilhada não é Git/Markdown-first e o autosave inclui conversas. ([github.com](https://github.com/supermemoryai/supermemory)) |
| **LongMemory** | **3,5/5** | **B+** | Aproximadamente 4,5 mil estrelas, 503 forks e 387 commits, com documentação de segurança, governança e dezenas de contribuidores visíveis. Usa SQLite local, conteúdo imutável, proveniência, temporalidade, recuperação explicável e contexto limitado por tokens. Seu escopo é amplo e não trata Markdown e Git como o único modelo canônico. ([github.com](https://github.com/CaviraOSS/OpenMemory)) |
| **Memorix** | **4,5/5** | **B+** | Aproximadamente 714 estrelas, 56 forks e 785 commits. Oferece memória entre diferentes coding agents, memória de projeto sob Git, revisão de afirmações e propostas que não sobrescrevem silenciosamente páginas revisadas. Usa SQLite como armazenamento canônico, mas apresenta um fluxo relevante de revisão e fronteiras de confiança. ([github.com](https://github.com/AVIDS2/memorix)) |

### Projetos próximos, porém ainda pequenos

Estes projetos não satisfazem o critério de maturidade para adoção como base, mas apresentam conceitos alinhados ao objeto da pesquisa:

| Projeto | Maturidade | Avaliação |
|---|---:|---|
| **mcncarl/agent-memory-vault** | **C+** | Markdown canônico, SQLite FTS derivado, uma única história Git, verificação de fontes, aprovações, receipts e uma sequência de prewrite, edição, dry-run e commit. Tem somente 40 commits e comunidade pequena. ([github.com](https://github.com/mcncarl/agent-memory-vault)) |
| **xChuCx/agent-memory** | **C+** | Usa Markdown, Git, `review --diff`, aplicação explícita, scan de segredos, merge driver para seções e federação de repositórios read-only fixados por commit. Tem cerca de 9 estrelas e 97 commits. ([github.com](https://github.com/xChuCx/agent-memory)) |
| **Vault-Agent-Memory** | **C+/B-** | Tem 830 commits, mas somente cerca de 48 estrelas. Implementa candidatos, revisão, promoção, auditoria e relatórios, mantendo trabalho sensível ou conflitante visível para revisão. O projeto ainda se apresenta como beta ou preview. ([github.com](https://github.com/zycaskevin/Vault-Agent-Memory)) |
| **observational-memory** | **C+** | Usa resumos nativos aprovados por Claude ou Codex em vez de transcrições, busca BM25, checagem de conflitos, proveniência e contexto inicial limitado. Tem aproximadamente 21 estrelas e comunidade pequena. ([github.com](https://github.com/intertwine/observational-memory)) |
| **Engram** | **C** | Oferece memória compartilhada para times em Markdown sob Git, tokens de leitura e escrita por agente, busca com `superseded_by` e `valid_until` e ausência de banco de dados obrigatório. Tem aproximadamente 11 estrelas. ([github.com](https://github.com/rwnalds/engram)) |
| **twiceshy** | **C** | Mantém registros propostos em quarentena e exige evidência, validação e PR humano para promoção. Inclui deduplicação, detecção de stale knowledge e reexecução de validações. Está em alpha e sem adoção significativa. ([github.com](https://github.com/dotts-h/twiceshy)) |

Também foram identificados `RepoMemo`, `agent-memory-engine`, `memo` e `ai-knowledge-base`. Todos possuem ideias relacionadas a bridges entre agentes, testes determinísticos, detecção de contradições e revisão interativa, mas apresentam comunidades muito pequenas ou limitações como ausência de memória compartilhada. ([github.com](https://github.com/SUN-1024/repomemo))

---

## Síntese do panorama

Os projetos analisados podem ser agrupados em cinco linhas principais:

| Linha | Projetos representativos | Características predominantes |
|---|---|---|
| **Markdown e arquivos locais** | Basic Memory, Palinode, EverOS, AI Memory Vault | Arquivos legíveis, organização por entidades ou páginas e índices locais derivados. |
| **Captura por hooks** | claude-mem, akitaonrails/ai-memory, MemPalace, memsearch | Integração com o ciclo de vida dos agentes, captura automática e recuperação contextual. |
| **Memória temporal e relacional** | Graphiti, MemoryBear, Mem0 | Validade temporal, relações entre fatos, contradições e atualização de conhecimento. |
| **Plataformas de memória** | Cognee, Supermemory, MemMachine, LongMemory | Serviços próprios, múltiplos armazenamentos e APIs de memória mais amplas. |
| **Governança baseada em Git** | xChuCx/agent-memory, agent-memory-vault, Vault-Agent-Memory, Engram, twiceshy | Propostas, revisão, validação, promoção e histórico auditável. |

Em termos de maturidade, os projetos com maior adoção concentram-se em captura de conversas, memória de agentes e plataformas apoiadas por bancos de dados ou serviços próprios. Os projetos mais alinhados a Markdown, Git, revisão explícita e memória compartilhada de domínio ainda apresentam comunidades menores ou estágio inicial de desenvolvimento.

Basic Memory, akitaonrails/ai-memory e Palinode são os projetos que mais se aproximam do conjunto de critérios da pesquisa. Graphiti, claude-mem, OpenViking e Mem0 se destacam em dimensões específicas, respectivamente temporalidade, captura por hooks, recuperação progressiva e memória factual. Os projetos menores concentram as experiências mais próximas de governança por propostas e revisão em Git.
