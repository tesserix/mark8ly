"use client";

import * as React from "react";
import {
  DndContext,
  PointerSensor,
  useSensor,
  useSensors,
  type DragEndEvent,
} from "@dnd-kit/core";
import {
  SortableContext,
  arrayMove,
  horizontalListSortingStrategy,
  useSortable,
} from "@dnd-kit/sortable";
import { CSS } from "@dnd-kit/utilities";
import type { AdminMediaResponse } from "@/lib/api/marketplace-api";
import { MediaCard, type MediaAction } from "./MediaCard";

export interface MediaGridProps {
  items: AdminMediaResponse[];
  onReorder: (nextOrder: AdminMediaResponse[]) => void;
  onAction: (action: MediaAction, media: AdminMediaResponse) => void;
}

interface SortableItemProps {
  media: AdminMediaResponse;
  isPrimary: boolean;
  onAction: (action: MediaAction, media: AdminMediaResponse) => void;
}

function SortableItem({ media, isPrimary, onAction }: SortableItemProps): React.ReactElement {
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } = useSortable({
    id: media.id,
  });
  const style: React.CSSProperties = {
    transform: CSS.Transform.toString(transform),
    transition,
    opacity: isDragging ? 0.5 : 1,
  };
  return (
    <li ref={setNodeRef} style={style} className="list-none" {...attributes} {...listeners}>
      <MediaCard media={media} isPrimary={isPrimary} onAction={onAction} />
    </li>
  );
}

export function MediaGrid({ items, onReorder, onAction }: MediaGridProps): React.ReactElement {
  const sensors = useSensors(useSensor(PointerSensor, { activationConstraint: { distance: 4 } }));

  const handleDragEnd = (event: DragEndEvent): void => {
    const { active, over } = event;
    if (!over || active.id === over.id) return;
    const oldIndex = items.findIndex((m) => m.id === active.id);
    const newIndex = items.findIndex((m) => m.id === over.id);
    if (oldIndex < 0 || newIndex < 0) return;
    onReorder(arrayMove(items, oldIndex, newIndex));
  };

  return (
    <DndContext sensors={sensors} onDragEnd={handleDragEnd}>
      <SortableContext items={items.map((m) => m.id)} strategy={horizontalListSortingStrategy}>
        <ol role="list" className="flex flex-wrap gap-4 p-0">
          {items.map((m, i) => (
            <SortableItem key={m.id} media={m} isPrimary={i === 0} onAction={onAction} />
          ))}
        </ol>
      </SortableContext>
    </DndContext>
  );
}
