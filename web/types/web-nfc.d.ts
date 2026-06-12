// Type declarations for the Web NFC API (https://w3c.github.io/web-nfc/),
// which is not included in TypeScript's standard DOM lib.

interface NDEFRecordInit {
  recordType: string;
  mediaType?: string;
  id?: string;
  encoding?: string;
  lang?: string;
  data?: string | BufferSource | NDEFMessageInit;
}

interface NDEFMessageInit {
  records: NDEFRecordInit[];
}

interface NDEFWriteOptions {
  overwrite?: boolean;
  signal?: AbortSignal;
}

interface NDEFScanOptions {
  signal?: AbortSignal;
}

interface NDEFRecord {
  readonly recordType: string;
  readonly mediaType: string | null;
  readonly id: string | null;
  readonly data: DataView | null;
  readonly encoding: string | null;
  readonly lang: string | null;
  toRecords?(): NDEFRecord[];
}

interface NDEFMessage {
  readonly records: ReadonlyArray<NDEFRecord>;
}

interface NDEFReadingEvent extends Event {
  readonly serialNumber: string;
  readonly message: NDEFMessage;
}

declare class NDEFReader extends EventTarget {
  constructor();
  onreading: ((this: NDEFReader, event: NDEFReadingEvent) => void) | null;
  onreadingerror: ((this: NDEFReader, event: Event) => void) | null;
  scan(options?: NDEFScanOptions): Promise<void>;
  write(
    message: string | BufferSource | NDEFMessageInit,
    options?: NDEFWriteOptions
  ): Promise<void>;
}

// biome-ignore lint/correctness/noUnusedVariables: global Window augmentation
interface Window {
  NDEFReader?: typeof NDEFReader;
}
