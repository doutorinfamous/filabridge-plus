"use client";

import { AlertTriangle } from "lucide-react";
import { toast } from "sonner";

import { api } from "@/lib/api";
import type { PrintError } from "@/lib/types";
import {
  Alert,
  AlertDescription,
  AlertTitle,
} from "@/components/ui/alert";
import { Button } from "@/components/ui/button";

export function PrintErrors({
  errors,
  onChanged,
}: {
  errors: PrintError[];
  onChanged: () => void;
}) {
  if (errors.length === 0) return null;

  const acknowledge = async (id: string) => {
    try {
      await api.acknowledgePrintError(id);
      toast.success("Erro reconhecido");
      onChanged();
    } catch (error) {
      toast.error(
        error instanceof Error ? error.message : "Falha ao reconhecer erro"
      );
    }
  };

  return (
    <div className="space-y-3">
      {errors.map((err) => (
        <Alert
          key={err.id}
          variant="destructive"
          className="border-destructive/40 bg-destructive/10"
        >
          <AlertTriangle className="size-4" />
          <AlertTitle>
            Falha ao processar impressão — {err.printer_name}
          </AlertTitle>
          <AlertDescription className="space-y-2 break-words">
            <p className="break-words">
              <span className="font-medium">Arquivo:</span> {err.filename}
              <span className="px-2 text-muted-foreground">·</span>
              <span className="font-medium">Quando:</span>{" "}
              {new Date(err.timestamp).toLocaleString()}
            </p>
            <p className="break-words">{err.error}</p>
            <p className="text-xs">
              Atualize o Spoolman manualmente com o uso correto de filamento
              desta impressão.
            </p>
            <Button
              variant="destructive"
              size="sm"
              onClick={() => acknowledge(err.id)}
            >
              Reconhecer
            </Button>
          </AlertDescription>
        </Alert>
      ))}
    </div>
  );
}
