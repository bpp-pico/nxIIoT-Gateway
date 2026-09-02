// Thousands-separated integer formatting for counters/durations shown in
// the UI (pending records, retry counts, poll durations, log attrs, etc.)
// — plain numbers like 218920 are hard to read at a glance once a queue
// backlog or a TX/RX counter climbs into the tens/hundreds of thousands.
export function fmtNum(v: number): string {
  return v.toLocaleString('en-US')
}
