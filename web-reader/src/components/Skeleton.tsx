// 骨架屏：静态色块，无动画。
export function Skeleton(props: { rows?: number; cover?: boolean; title?: boolean }) {
  const rows = props.rows ?? 6;
  return (
    <div className="skeleton-block" aria-hidden>
      {props.title ? <div className="skeleton title" /> : null}
      {props.cover ? <div className="skeleton cover" /> : null}
      {Array.from({ length: rows }, (_, i) => (
        <div className="skeleton" key={i} />
      ))}
    </div>
  );
}
