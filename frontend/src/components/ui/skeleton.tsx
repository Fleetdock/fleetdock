/** Skeleton is a placeholder block shown while content loads. */
export function Skeleton({ width = "100%", height = "1rem" }: { width?: string | number; height?: string | number }) {
  return <span className="skeleton" style={{ width, height }} aria-hidden />;
}

/** TableSkeleton mimics a table while its rows load. */
export function TableSkeleton({ rows = 5, columns = 4 }: { rows?: number; columns?: number }) {
  return (
    <div className="card table-wrap" role="status" aria-label="Loading">
      <table className="table">
        <tbody>
          {Array.from({ length: rows }, (_, r) => (
            <tr key={r}>
              {Array.from({ length: columns }, (_, c) => (
                <td key={c}>
                  <Skeleton width={c === 0 ? "60%" : "40%"} />
                </td>
              ))}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
