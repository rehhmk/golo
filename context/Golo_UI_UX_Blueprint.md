# Blueprint de Identidade Visual, UI & UX: Golo
*Plataforma Analytics de Probabilidade de Gol em Tempo Real (MVP)*

---

## 1. Posicionamento e Sentimento da Interface (UI Vibe)

### O Sentimento: **Urgência Analítica Profissional**
O **Golo** adota uma estética *Data-Dense Dark Mode*, combinando a seriedade de uma mesa de trading financeiro (Bloomberg, TradingView) com o dinamismo e a tensão do futebol ao vivo.

* **Precisão & Autoridade:** Informação fria, baseada em dados estatísticos ($xG$, pressão, chutes no alvo), sem viés torcedor ou ruído decorativo.
* **Urgência & Tensão:** Destaques em alta intensidade neon para sinalizar oportunidades e momentos críticos da partida em tempo real.
* **Clareza Instantânea:** Leitura de relance em menos de 3 segundos ("Glanceability") — o usuário bate o olho e identifica onde está o valor.

---

## 2. Nome & Marca

* **Nome oficial:** **Golo**
* **Slogan / Tagline:** *Real-Time Football Probability Engine*
* **Símbolo / Ícone:** Um pulso eletrocardiograma estilizado intersecção com a silhueta geométrica de uma bola de futebol ou uma linha de meta, simbolizando o "batimento" do jogo.

---

## 3. Identidade Visual & Design System

### Paleta de Cores (Tailwind CSS Native)

| Aplicação | Cor | HEX | Função UX / Significado |
| :--- | :--- | :--- | :--- |
| **Canvas / Background Main** | Slate 950 | `#020617` | Fundo ultra-escuro para contraste máximo, economia de OLED e conforto visual em ambientes escuros. |
| **Card / Container Surface** | Slate 900 | `#0F172A` | Elevação de camadas UI, agrupamento lógico de estatísticas por partida. |
| **Bordas & Divisores** | Slate 800 | `#1E293B` | Separação sutil sem poluição visual. |
| **Probabilidade Alta (Hot)** | Emerald 400 | `#34D399` | **Sinal Verde:** Probabilidade de gol elevada ($> 75\%$), zona de pressão máxima no jogo. |
| **Probabilidade Média (Warm)**| Amber 400 | `#FBBF24` | **Alerta:** Aumento de intensidade, oportunidade em gestação ($45\% - 74\%$). |
| **Probabilidade Baixa (Neutral)**| Slate 400 | `#94A3B8` | Estado neutro de jogo truncado ($< 44\%$). |
| **Live Indicator / Urgência**| Rose 500 | `#F43F5E` | Badge de tempo real, cartões vermelhos, momentos críticos instantâneos. |
| **Texto Principal** | Slate 50 | `#F8FAFC` | Leitura de alto contraste para métricas principais. |
| **Texto Secundário** | Slate 400 | `#94A3B8` | Rótulos, horários e metadados secundários. |

### Tipografia

1. **Interface & Títulos (UI Font):**
   * **Fonte:** `Inter` ou `Plus Jakarta Sans`
   * **Uso:** Nomes de times, navegação, menus, rótulos e botões.
   * **Característica:** Excelente legibilidade em telas de alta densidade e tamanhos pequenos.

2. **Números & Métricas (Data Font):**
   * **Fonte:** `JetBrains Mono` ou `Roboto Mono`
   * **Uso:** Porcentagens de probabilidade, placar, minutagem, dados de $xG$.
   * **Por que tabular/monospaced?:** Impede a variação de largura na tela quando os números e decimais mudam ao vivo via WebSocket/Server-Sent Events.

---

## 4. Stack Tecnológica de UI (Zero Cost & Fast MVP)

| Camada | Tecnologia | Implementação |
| :--- | :--- | :--- |
| **Framework Base** | **React** (Next.js ou Vite) | Componentização e SSR/Client rendering ultra-rápido. |
| **Estilização** | **Tailwind CSS** | Utilitários diretos, paleta nativa `slate`, performance zero-runtime. |
| **Componentes UI** | **Shadcn UI** | Cópia-e-cola de primitivos Radix UI (Cards, Badges, Tabs, Dialogs, Tooltips) 100% customizáveis. |
| **Gráficos em Tempo Real** | **Recharts** ou **Tremor** | **Recharts AreaChart / Sparklines** para mostrar a curva de pressão dos últimos 15 minutos. |
| **Ícones** | **Lucide React** | Ícones limpos e alinhados com o ecossistema Shadcn. |
| **Animações / Feedback** | **Framer Motion** | Micro-animações para transições de valores percentuais e pulso visual. |

---

## 5. Arquitetura de UX & Disposição de Layout

### A. Painel Principal (Dashboard List View)

A tela inicial organiza os jogos por **Grau de Oportunidade / Probabilidade de Gol**, e não por liga, priorizando valor imediato para o usuário.

#### Estrutura do Card de Partida (Game Card Component)
```text
+-----------------------------------------------------------------------------------+
| [LIVE 68'] Premier League                             [PROB. GOL (10m): 82%]    |
| Arsenal  1 - 1  Chelsea                                  HIGH PRESSURE (EMERALD)|
+-----------------------------------------------------------------------------------+
|  GRÁFICO SPARKLINE (PRESSÃO NOS ÚLTIMOS 15 MINUTOS)                               |
|  [~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~/^\~~~~~~~~]        |
+-----------------------------------------------------------------------------------+
|  Métricas Chave:                                                                  |
|  xG Acumulado: 2.14   |   Chutes no Alvo (U10): 4   |   Escanteios (U10): 3     |
+-----------------------------------------------------------------------------------+
```

### B. Regras de Micro-Interações de UX

1. **Pulse Indicator:** O badge de probabilidade pisca suavemente em `Emerald 400` quando a probabilidade cruza o limiar de $75\%$.
2. **Flash de Mudança de Estado:** Quando a probabilidade sobe mais de $10\%$ em uma única atualização, o card aplica um efeito sutil de *border highlight* temporário.
3. **Smart Sorting:** Toggle para ordenar os jogos por:
   * **Maior Probabilidade Agora** (Default)
   * **Maior Variação nos Últimos 5 min** (*Momentum*)
   * **Por Liga / Campeonato**

---

## 6. Exemplo Prático de Código (Shadcn + Tailwind Component)

```tsx
import React from 'react';
import { Activity, Flame } from 'lucide-react';

interface GameCardProps {
  homeTeam: string;
  awayTeam: string;
  homeScore: number;
  awayScore: number;
  minute: number;
  league: string;
  probability: number; // 0 to 100
}

export const GameCard: React.FC<GameCardProps> = ({
  homeTeam,
  awayTeam,
  homeScore,
  awayScore,
  minute,
  league,
  probability,
}) => {
  const isHighProb = probability >= 75;
  const isMediumProb = probability >= 45 && probability < 75;

  const statusColor = isHighProb
    ? 'text-emerald-400 border-emerald-500/30 bg-emerald-500/10'
    : isMediumProb
    ? 'text-amber-400 border-amber-500/30 bg-amber-500/10'
    : 'text-slate-400 border-slate-700 bg-slate-800/40';

  return (
    <div className="w-full bg-slate-900 border border-slate-800 rounded-xl p-4 hover:border-slate-700 transition-all shadow-lg">
      {/* Header */}
      <div className="flex justify-between items-center mb-3">
        <div className="flex items-center gap-2">
          <span className="flex items-center gap-1 text-xs font-semibold text-rose-500 bg-rose-500/10 px-2 py-0.5 rounded-full border border-rose-500/20 animate-pulse">
            <span className="w-1.5 h-1.5 rounded-full bg-rose-500"></span>
            {minute}'
          </span>
          <span className="text-xs text-slate-400 font-medium">{league}</span>
        </div>
        {isHighProb && (
          <span className="flex items-center gap-1 text-xs font-bold text-emerald-400 bg-emerald-500/10 px-2 py-0.5 rounded-full border border-emerald-500/20">
            <Flame className="w-3.5 h-3.5" /> HOT MATCH
          </span>
        )}
      </div>

      {/* Score & Match */}
      <div className="flex justify-between items-center my-2">
        <div className="space-y-1 font-semibold text-slate-100 text-sm">
          <div>{homeTeam}</div>
          <div>{awayTeam}</div>
        </div>
        <div className="font-mono text-lg font-bold text-slate-200 bg-slate-950 px-3 py-1 rounded-lg border border-slate-800">
          {homeScore} - {awayScore}
        </div>
      </div>

      {/* Probabilities Metric Block */}
      <div className="mt-4 pt-3 border-t border-slate-800/80 flex items-center justify-between">
        <div className="flex items-center gap-1.5 text-xs text-slate-400">
          <Activity className="w-4 h-4 text-slate-500" />
          <span>Prob. de Gol (Próx. 10m)</span>
        </div>
        <div className={`font-mono text-xl font-extrabold px-2.5 py-1 rounded-lg border ${statusColor}`}>
          {probability}%
        </div>
      </div>
    </div>
  );
};
