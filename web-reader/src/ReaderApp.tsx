import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { BrowserRouter, Route, Routes } from "react-router-dom";
import { SearchPage } from "./pages/SearchPage";
import { BookPage } from "./pages/BookPage";
import { ReaderPage } from "./pages/ReaderPage";
import "./reader.css";

const queryClient = new QueryClient();

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
