# go-ord-tx

## golang ordinals btc nft inscribe tx

### Supports two types of batch inscriptions:

- One commit transaction with multiple reveal transactions.
- One commit transaction with a single reveal transaction.

## Inscription Examples

The inscribewithoutnoderpc, inscribewithprivatenoderpc, and inscribewithpublicnoderpc examples demonstrate how to use the go-ord-tx package to create inscriptions.

- inscribewithoutnoderpc: sends raw transactions directly to the network, without using an RPC node;
- inscribewithprivatenoderpc: uses a private RPC node.
- inscribewithpublicnoderpc: uses a public RPC node.
