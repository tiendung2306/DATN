import { create } from 'zustand'
import { service } from '../../wailsjs/go/models'

export type NetworkConnectionState =
  | 'offline'
  | 'starting'
  | 'connected'
  | 'syncing'
  | 'authorized_no_peers'
  | 'reconnecting'

interface NetworkState {
  status: NetworkConnectionState
  connectedPeers: service.PeerInfo[]
  localPeerId: string
  setStatus: (status: NetworkConnectionState) => void
  setConnectedPeers: (peers: service.PeerInfo[]) => void
  setLocalPeerId: (peerId: string) => void
  reset: () => void
}

export const useNetworkStore = create<NetworkState>((set) => ({
  status: 'starting',
  connectedPeers: [],
  localPeerId: '',
  setStatus: (status) => set({ status }),
  setConnectedPeers: (peers) => set({ connectedPeers: peers }),
  setLocalPeerId: (peerId) => set({ localPeerId: peerId }),
  reset: () => set({ status: 'starting', connectedPeers: [], localPeerId: '' }),
}))
