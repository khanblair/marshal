import { Menu, MenuItem } from "@marshal/ui";
import { askRemoveProject, openProjectSettings, startRename } from "./sidebar-actions";

export interface ProjectMenuProps {
  id: string;
}

/** The sidebar row's actions menu. It closes through the store, like the design's. */
export function ProjectMenu(props: ProjectMenuProps) {
  return (
    <Menu class="absolute right-0 top-[calc(100%+2px)] w-[200px] z-[170]">
      <MenuItem icon="pencil" onClick={() => startRename(props.id)}>
        Rename
      </MenuItem>
      <MenuItem icon="settings-2" onClick={() => openProjectSettings(props.id)}>
        Project settings
      </MenuItem>
      <MenuItem icon="folder-x" danger onClick={() => askRemoveProject(props.id)}>
        Remove
      </MenuItem>
    </Menu>
  );
}
