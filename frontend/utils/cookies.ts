import { getCookie, setCookie, deleteCookie } from 'cookies-next'

export const Cookies = {
  get: (key: string) => getCookie(key),
  set: (key: string, value: string, maxAge = 86400) =>
    setCookie(key, value, { maxAge }),
  delete: (key: string) => deleteCookie(key),
}
