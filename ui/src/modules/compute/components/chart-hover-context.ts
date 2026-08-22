import { createContext } from "react"

/**
 * The timestamp under the cursor, shared by every chart in a metrics
 * panel so they can all draw the crosshair at the same instant.
 *
 * It travels by context rather than as a prop because the charts are
 * memoised: a prop would change on every mousemove and re-render all
 * six AreaCharts (twenty-odd paths each), which is exactly the stutter
 * this replaced. Only the crosshair subscribes, so a mousemove costs
 * six style updates instead of six chart redraws.
 *
 * Its own file because the chart module exports components, and
 * react-refresh only keeps fast refresh working when a component file
 * exports nothing else.
 */
export const HoverTsContext = createContext<number | null>(null)
