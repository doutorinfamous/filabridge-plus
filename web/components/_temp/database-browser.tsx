"use client";

import * as React from "react";
import {
  ChevronLeft,
  ChevronRight,
  Database,
  RefreshCw,
  Search,
} from "lucide-react";
import { toast } from "sonner";

import { api } from "@/lib/api";
import type { DevDbTable, DevDbTableData } from "@/lib/types";
import { cn } from "@/lib/utils";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { ScrollArea } from "@/components/ui/scroll-area";
import { Skeleton } from "@/components/ui/skeleton";

const PAGE_SIZE = 100;
const REFRESH_MS = 5000;

function formatCell(value: unknown): string {
  if (value == null) return "NULL";
  if (typeof value === "object") return JSON.stringify(value);
  return String(value);
}

function formatTime(date: Date): string {
  return date.toLocaleTimeString("pt-BR", {
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
  });
}

function SchemaTable({ data }: { data: DevDbTableData }) {
  const schema = data.schema ?? [];
  if (schema.length === 0) return null;

  return (
    <div className="mb-4 space-y-2">
      <h4 className="text-xs font-medium uppercase tracking-wide text-muted-foreground">
        Estrutura
      </h4>
      <div className="overflow-x-auto rounded-lg border border-border">
        <table className="w-max min-w-full border-collapse text-xs">
          <thead>
            <tr className="border-b border-border bg-muted/40">
              <th className="whitespace-nowrap px-3 py-2 text-left font-medium text-muted-foreground">
                Coluna
              </th>
              <th className="whitespace-nowrap px-3 py-2 text-left font-medium text-muted-foreground">
                Tipo
              </th>
              <th className="whitespace-nowrap px-3 py-2 text-left font-medium text-muted-foreground">
                PK
              </th>
              <th className="whitespace-nowrap px-3 py-2 text-left font-medium text-muted-foreground">
                NOT NULL
              </th>
              <th className="whitespace-nowrap px-3 py-2 text-left font-medium text-muted-foreground">
                Default
              </th>
            </tr>
          </thead>
          <tbody>
            {schema.map((col) => (
              <tr
                key={col.name}
                className="border-b border-border/60 hover:bg-muted/20"
              >
                <td className="whitespace-nowrap px-3 py-2 font-mono font-medium">
                  {col.name}
                </td>
                <td className="whitespace-nowrap px-3 py-2 font-mono text-muted-foreground">
                  {col.type}
                </td>
                <td className="whitespace-nowrap px-3 py-2">
                  {col.primary_key ? (
                    <Badge variant="outline" className="text-[10px]">
                      PK
                    </Badge>
                  ) : (
                    "—"
                  )}
                </td>
                <td className="whitespace-nowrap px-3 py-2">
                  {col.not_null ? "sim" : "—"}
                </td>
                <td
                  className="max-w-[240px] truncate whitespace-nowrap px-3 py-2 font-mono text-muted-foreground"
                  title={col.default_value}
                >
                  {col.default_value ?? "—"}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}

export function DatabaseBrowser() {
  const [tables, setTables] = React.useState<DevDbTable[] | null>(null);
  const [tableSearch, setTableSearch] = React.useState("");
  const [selectedTable, setSelectedTable] = React.useState<string | null>(null);
  const [tableData, setTableData] = React.useState<DevDbTableData | null>(null);
  const [offset, setOffset] = React.useState(0);
  const [lastUpdated, setLastUpdated] = React.useState<Date | null>(null);
  const [refreshing, setRefreshing] = React.useState(false);
  const [paginating, setPaginating] = React.useState(false);

  const refreshTables = React.useCallback(() => {
    return api
      .getDevDbTables()
      .then((res) => {
        setTables(res.tables ?? []);
        setLastUpdated(new Date());
      })
      .catch((error) => {
        toast.error(
          error instanceof Error ? error.message : "Falha ao carregar tabelas"
        );
      });
  }, []);

  const loadTableData = React.useCallback((name: string, nextOffset: number) => {
    return api
      .getDevDbTableData(name, PAGE_SIZE, nextOffset)
      .then((data) => {
        setTableData(data);
        setOffset(nextOffset);
        setLastUpdated(new Date());
      })
      .catch((error) => {
        setTableData(null);
        toast.error(
          error instanceof Error ? error.message : "Falha ao carregar dados"
        );
      });
  }, []);

  // Polling em tempo real: lista de tabelas
  React.useEffect(() => {
    refreshTables();
    const id = window.setInterval(() => refreshTables(), REFRESH_MS);
    return () => window.clearInterval(id);
  }, [refreshTables]);

  // Polling em tempo real: dados da tabela selecionada
  React.useEffect(() => {
    if (!selectedTable) return;
    loadTableData(selectedTable, offset);
    const id = window.setInterval(
      () => loadTableData(selectedTable, offset),
      REFRESH_MS
    );
    return () => window.clearInterval(id);
  }, [selectedTable, offset, loadTableData]);

  const selectTable = (name: string) => {
    setSelectedTable(name);
    setTableData(null);
    setOffset(0);
  };

  const handleManualRefresh = () => {
    setRefreshing(true);
    const tasks: Promise<unknown>[] = [refreshTables()];
    if (selectedTable) {
      tasks.push(loadTableData(selectedTable, offset));
    }
    Promise.all(tasks).finally(() => setRefreshing(false));
  };

  const showDataSkeleton =
    selectedTable != null &&
    (tableData == null || tableData.table !== selectedTable);

  const filteredTables = (tables ?? []).filter((table) => {
    if (!tableSearch.trim()) return true;
    return table.name.toLowerCase().includes(tableSearch.trim().toLowerCase());
  });

  const total = tableData?.total ?? 0;
  const canPrev = offset > 0;
  const canNext = tableData != null && offset + PAGE_SIZE < total;

  return (
    <div className="space-y-4">
      <Alert className="border-amber-500/30 bg-amber-500/10">
        <AlertDescription className="text-amber-200/90">
          Ferramenta temporária de debug — somente leitura. Atualiza
          automaticamente a cada {REFRESH_MS / 1000}s.
        </AlertDescription>
      </Alert>

      <div className="flex flex-wrap items-center justify-between gap-2">
        <p className="text-xs text-muted-foreground">
          {lastUpdated
            ? `Última atualização: ${formatTime(lastUpdated)}`
            : "Carregando…"}
        </p>
        <Button
          type="button"
          variant="outline"
          size="sm"
          disabled={refreshing || paginating}
          onClick={handleManualRefresh}
        >
          <RefreshCw
            className={cn(
              "size-4",
              (refreshing || paginating) && "animate-spin"
            )}
          />
          Atualizar agora
        </Button>
      </div>

      <div className="grid gap-4 lg:grid-cols-[minmax(0,4fr)_minmax(0,8fr)]">
        <Card className="border-border/70 bg-card/60 py-0">
          <CardContent className="p-3">
            <div className="relative mb-3">
              <Search className="absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
              <Input
                value={tableSearch}
                onChange={(e) => setTableSearch(e.target.value)}
                placeholder="Filtrar tabelas..."
                className="pl-9"
              />
            </div>
            <ScrollArea className="h-[520px] pr-2">
              {tables == null ? (
                <div className="space-y-2">
                  {["a", "b", "c", "d", "e", "f"].map((id) => (
                    <Skeleton key={id} className="h-11 rounded-lg" />
                  ))}
                </div>
              ) : filteredTables.length === 0 ? (
                <p className="px-2 py-8 text-center text-sm text-muted-foreground">
                  Nenhuma tabela encontrada.
                </p>
              ) : (
                <div className="space-y-1">
                  {filteredTables.map((table) => {
                    const isSelected = selectedTable === table.name;
                    return (
                      <button
                        type="button"
                        key={table.name}
                        onClick={() => selectTable(table.name)}
                        className={cn(
                          "flex w-full items-center justify-between gap-2 rounded-lg border px-3 py-2.5 text-left transition-colors",
                          isSelected
                            ? "border-primary/40 bg-accent"
                            : "border-transparent hover:bg-accent/50"
                        )}
                      >
                        <span className="truncate font-mono text-sm">
                          {table.name}
                        </span>
                        <Badge variant="secondary" className="shrink-0">
                          {table.row_count}
                        </Badge>
                      </button>
                    );
                  })}
                </div>
              )}
            </ScrollArea>
          </CardContent>
        </Card>

        <Card className="border-border/70 bg-card/60">
          <CardContent className="flex min-h-[560px] flex-col p-4">
            {!selectedTable ? (
              <div className="flex flex-1 flex-col items-center justify-center text-muted-foreground">
                <Database className="mb-3 size-10 opacity-40" />
                <p className="text-sm font-medium">Selecione uma tabela</p>
                <p className="text-xs">
                  Escolha uma tabela à esquerda para ver estrutura e dados
                </p>
              </div>
            ) : (
              <>
                <div className="mb-3 flex flex-wrap items-center justify-between gap-2">
                  <div>
                    <h3 className="font-mono text-sm font-semibold">
                      {selectedTable}
                    </h3>
                    <p className="text-xs text-muted-foreground">
                      {total} linha{total === 1 ? "" : "s"} · somente leitura
                    </p>
                  </div>
                  <div className="flex items-center gap-2">
                    <Button
                      type="button"
                      variant="outline"
                      size="sm"
                      disabled={!canPrev || paginating}
                      onClick={() => {
                        setPaginating(true);
                        loadTableData(selectedTable, offset - PAGE_SIZE).finally(
                          () => setPaginating(false)
                        );
                      }}
                    >
                      <ChevronLeft className="size-4" />
                      Anterior
                    </Button>
                    <span className="text-xs text-muted-foreground">
                      {total === 0
                        ? "0"
                        : `${offset + 1}–${Math.min(offset + PAGE_SIZE, total)}`}
                    </span>
                    <Button
                      type="button"
                      variant="outline"
                      size="sm"
                      disabled={!canNext || paginating}
                      onClick={() => {
                        setPaginating(true);
                        loadTableData(selectedTable, offset + PAGE_SIZE).finally(
                          () => setPaginating(false)
                        );
                      }}
                    >
                      Próximo
                      <ChevronRight className="size-4" />
                    </Button>
                  </div>
                </div>

                {showDataSkeleton ? (
                  <div className="space-y-2">
                    <Skeleton className="h-24 w-full" />
                    <Skeleton className="h-8 w-full" />
                    <Skeleton className="h-32 w-full" />
                  </div>
                ) : tableData ? (
                  <div className="flex min-h-0 flex-1 flex-col gap-4">
                    <SchemaTable data={tableData} />

                    <div className="flex min-h-0 flex-1 flex-col space-y-2">
                      <h4 className="text-xs font-medium uppercase tracking-wide text-muted-foreground">
                        Dados
                      </h4>
                      {tableData.columns.length > 0 ? (
                        <div className="min-h-0 flex-1 overflow-auto rounded-lg border border-border">
                          <div className="overflow-x-auto">
                            <table className="w-max min-w-full border-collapse text-xs">
                              <thead className="sticky top-0 z-10 bg-muted/90 backdrop-blur-sm">
                                <tr className="border-b border-border">
                                  {tableData.columns.map((col) => (
                                    <th
                                      key={col}
                                      className="whitespace-nowrap px-3 py-2 text-left font-medium text-muted-foreground"
                                    >
                                      {col}
                                    </th>
                                  ))}
                                </tr>
                              </thead>
                              <tbody>
                                {tableData.rows.length === 0 ? (
                                  <tr>
                                    <td
                                      colSpan={tableData.columns.length}
                                      className="px-3 py-8 text-center text-muted-foreground"
                                    >
                                      Tabela vazia.
                                    </td>
                                  </tr>
                                ) : (
                                  tableData.rows.map((row, rowIdx) => (
                                    <tr
                                      key={`${selectedTable}-row-${offset + rowIdx}`}
                                      className="border-b border-border/60 hover:bg-muted/20"
                                    >
                                      {tableData.columns.map((col) => (
                                        <td
                                          key={col}
                                          className="whitespace-nowrap px-3 py-2 font-mono"
                                          title={formatCell(row[col])}
                                        >
                                          {formatCell(row[col])}
                                        </td>
                                      ))}
                                    </tr>
                                  ))
                                )}
                              </tbody>
                            </table>
                          </div>
                        </div>
                      ) : (
                        <p className="text-sm text-muted-foreground">
                          Nenhuma coluna encontrada.
                        </p>
                      )}
                    </div>
                  </div>
                ) : (
                  <p className="text-sm text-muted-foreground">
                    Falha ao carregar a tabela.
                  </p>
                )}
              </>
            )}
          </CardContent>
        </Card>
      </div>
    </div>
  );
}
