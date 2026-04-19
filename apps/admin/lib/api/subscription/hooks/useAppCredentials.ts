/**
 * React Query hooks for Apple and Google credential uploads.
 *
 * Both mutations invalidate ['subscription', storeId] on success so the
 * lifecycle status widget on the billing page refreshes automatically.
 */
'use client'

import { useMutation, useQueryClient } from '@tanstack/react-query'
import type { UseMutationResult } from '@tanstack/react-query'
import { uploadAppleCredentials, uploadGoogleCredentials } from '../appCredentials'
import type { AppleCredentialsForm, GoogleCredentialsForm } from '../schemas/appCredentials'

/**
 * Mutation hook for uploading Apple App Store Connect credentials.
 *
 * @param storeId - The store UUID.
 */
export function useUploadAppleCredentials(
  storeId: string,
): UseMutationResult<void, Error, AppleCredentialsForm> {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: (fields: AppleCredentialsForm) =>
      uploadAppleCredentials(storeId, fields),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['subscription', storeId] })
    },
  })
}

/**
 * Mutation hook for uploading Google Play service account credentials.
 *
 * @param storeId - The store UUID.
 */
export function useUploadGoogleCredentials(
  storeId: string,
): UseMutationResult<void, Error, GoogleCredentialsForm> {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: (fields: GoogleCredentialsForm) =>
      uploadGoogleCredentials(storeId, fields),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['subscription', storeId] })
    },
  })
}
