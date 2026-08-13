export type HorizontalScrollTarget = {
  getBoundingClientRect: () => Pick<DOMRect, 'left' | 'right'>
}

export type HorizontalScrollContainer = HorizontalScrollTarget & {
  scrollBy: (options: ScrollToOptions) => void
}

export function revealHorizontalTarget(
  container: HorizontalScrollContainer,
  target: HorizontalScrollTarget
) {
  const containerRect = container.getBoundingClientRect()
  const targetRect = target.getBoundingClientRect()
  let left = 0
  if (targetRect.left < containerRect.left) {
    left = targetRect.left - containerRect.left
  } else if (targetRect.right > containerRect.right) {
    left = targetRect.right - containerRect.right
  }
  if (left !== 0) container.scrollBy({ behavior: 'auto', left })
}
