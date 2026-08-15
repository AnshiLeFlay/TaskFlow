import { defineStore } from 'pinia'

export type ToastTone = 'success' | 'error' | 'info'

export interface Toast {
  id: number
  title: string
  message?: string
  tone: ToastTone
}

export const useToastStore = defineStore('toasts', {
  state: () => ({ items: [] as Toast[], nextId: 1 }),
  actions: {
    show(title: string, options: { message?: string; tone?: ToastTone; timeout?: number } = {}) {
      const id = this.nextId++
      this.items.push({ id, title, message: options.message, tone: options.tone || 'info' })
      window.setTimeout(() => this.dismiss(id), options.timeout ?? 4200)
      return id
    },
    dismiss(id: number) {
      this.items = this.items.filter((toast) => toast.id !== id)
    },
  },
})
