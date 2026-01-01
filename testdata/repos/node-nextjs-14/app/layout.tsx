import type { Metadata } from 'next'

export const metadata: Metadata = {
  title: 'Next.js 14 App',
  description: 'A Next.js 14 application with App Router',
}

export default function RootLayout({
  children,
}: {
  children: React.ReactNode
}) {
  return (
    <html lang="en">
      <body>{children}</body>
    </html>
  )
}
