import { PrintHistory } from "@/components/print-history";

export default function HistoryPage() {
  return (
    <div className="space-y-6">
      <header>
        <h1 className="text-2xl font-semibold tracking-tight">Print History</h1>
        <p className="text-sm text-muted-foreground">
          Recorded prints with filament usage per toolhead/tray and spool
        </p>
      </header>
      <PrintHistory />
    </div>
  );
}
