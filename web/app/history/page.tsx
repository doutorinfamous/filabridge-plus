import { PrintHistory } from "@/components/print-history";

export default function HistoryPage() {
  return (
    <div className="space-y-6">
      <header>
        <h1 className="text-2xl font-semibold tracking-tight">
          Histórico de Impressão
        </h1>
        <p className="text-sm text-muted-foreground">
          Impressões registradas com consumo de filamento por toolhead/tray e
          spool
        </p>
      </header>
      <PrintHistory />
    </div>
  );
}
