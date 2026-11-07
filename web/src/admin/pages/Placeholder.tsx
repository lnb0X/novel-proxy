import { Empty } from "antd";
import { useLocation } from "react-router-dom";

// W13 页面占位：骨架期保证路由与导航可用。
export function PlaceholderPage() {
  const location = useLocation();
  return (
    <div>
      <Empty description={`「${location.pathname}」页面建设中`} />
    </div>
  );
}
