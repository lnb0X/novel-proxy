import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { BrowserRouter, Route, Routes } from "react-router-dom";
import { SearchPage } from "./pages/SearchPage";
import { BookPage } from "./pages/BookPage";
import { ReaderPage } from "./pages/ReaderPage";
import "./tokens.css";
import "./reader.css";

// 阅读端快速失败：上游错误不重试，错误态立即展示。
const queryClient = new QueryClient({
  defaultOptions: { queries: { retry: false } },
});

export function ReaderApp() {
  return (
    <QueryClientProvider client={queryClient}>
      <BrowserRouter>
        <Routes>
          <Route path="/" element={<SearchPage />} />
          <Route path="/books/:bookId" element={<BookPage />} />
          <Route path="/books/:bookId/read/:chapterId" element={<ReaderPage />} />
        </Routes>
      </BrowserRouter>
    </QueryClientProvider>
  );
}
