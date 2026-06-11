import { DatabaseBrowser } from "@/components/_temp/database-browser";

export default function DevDatabasePage() {
  return (
    <div className="space-y-6">
      <header>
        <h1 className="text-2xl font-semibold tracking-tight">Banco de Dados</h1>
        <p className="text-sm text-muted-foreground">
          Inspeção read-only do SQLite local (filabridge.db), com estrutura e
          dados atualizados em tempo real
        </p>
      </header>
      <DatabaseBrowser />
    </div>
  );
}
