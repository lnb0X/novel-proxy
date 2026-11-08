// 错误重试：文案区分上游失败，点击重试重新发起请求。
export function ErrorRetry(props: { message: string; onRetry: () => void }) {
  return (
    <div>
      <p className="muted">{props.message}</p>
      <button className="btn" onClick={props.onRetry}>
        重试
      </button>
    </div>
  );
}
