import { exportBoardToSheet, exportBoardToSlides, exportKey } from "./export-actions";
import { isExporting } from "./export-run";

/** One way to send a board to Google. The menu that shows it closes itself before it runs `run`. */
export interface ProjectExportItem {
  label: string;
  icon: string;
  /** True while this export of this project already runs. */
  disabled: boolean;
  run: () => void;
}

/** The two exports of a project's board, for the project's More menu and the phone's More sheet. */
export function projectExportItems(pid: string): ProjectExportItem[] {
  return [
    {
      label: "Export board to Google Sheet",
      icon: "columns-3",
      disabled: isExporting(exportKey("sheet", pid)),
      run: () => void exportBoardToSheet(pid),
    },
    {
      label: "Export board to Google Slides",
      icon: "monitor",
      disabled: isExporting(exportKey("slides", pid)),
      run: () => void exportBoardToSlides(pid),
    },
  ];
}
