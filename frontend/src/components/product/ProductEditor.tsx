import { useEffect, useRef, useState } from 'react';
import { Alert, Button, Stack, TextInput } from '@mantine/core';
import { getProduct, updateProduct } from '../../api/client';
import type { ProductDetail, ProductWriteInput } from '../../types';
import { ContributeFields, type ContributeChoice } from './ContributeFields';
import { showShareNotice } from './shareNotice';

export interface ProductEditorProps {
  productId: string;
  onSaved: () => void;
}

const emptyChoice: ContributeChoice = {
  contribute: false,
  contributeTo: 'openfoodfacts',
};

export const ProductEditor = ({ productId, onSaved }: ProductEditorProps) => {
  const [open, setOpen] = useState(false);
  const [product, setProduct] = useState<ProductDetail | null>(null);
  const [name, setName] = useState('');
  const [category, setCategory] = useState('');
  const [unitOfMeasure, setUnitOfMeasure] = useState('');
  const [barcode, setBarcode] = useState('');
  const [share, setShare] = useState<ContributeChoice>(emptyChoice);
  const [error, setError] = useState('');
  const [saving, setSaving] = useState(false);
  // A slow product read must not replace text the person has already typed.
  const draftDirty = useRef(false);

  useEffect(() => {
    if (!open) {
      draftDirty.current = false;
      return;
    }
    let active = true;
    void getProduct(productId)
      .then((loaded) => {
        if (!active || draftDirty.current) return;
        setProduct(loaded);
        setName(loaded.name);
        setCategory(loaded.category);
        setUnitOfMeasure(loaded.unitOfMeasure);
        setBarcode(loaded.barcodes[0] ?? '');
      })
      .catch((requestError: unknown) => {
        if (active) {
          setError(requestError instanceof Error ? requestError.message : 'Unable to load product.');
        }
      });
    return () => {
      active = false;
    };
  }, [open, productId]);

  if (!open) {
    return (
      <Button size="xs" variant="light" onClick={() => setOpen(true)}>
        Edit product
      </Button>
    );
  }

  const upstream = product?.source === 'external' || Boolean(product?.externalSource);

  const save = async () => {
    if (name.trim() === '') return;
    setSaving(true);
    setError('');
    const input: ProductWriteInput = {
      name: name.trim(),
      category,
      unitOfMeasure,
    };
    if (share.contribute) {
      input.contribute = true;
      input.contributeTo = share.contributeTo;
      input.barcode = barcode.trim();
    }
    try {
      const saved = await updateProduct(productId, input);
      showShareNotice(saved);
      onSaved();
      setOpen(false);
    } catch (requestError) {
      setError(requestError instanceof Error ? requestError.message : 'Unable to save product.');
    } finally {
      setSaving(false);
    }
  };

  return (
    <Stack gap="xs">
      <TextInput
        label="Product name"
        required
        value={name}
        onChange={(event) => {
          draftDirty.current = true;
          setName(event.currentTarget.value);
        }}
      />
      <TextInput
        label="Category"
        value={category}
        onChange={(event) => {
          draftDirty.current = true;
          setCategory(event.currentTarget.value);
        }}
      />
      <TextInput
        label="Unit of measure"
        value={unitOfMeasure}
        onChange={(event) => {
          draftDirty.current = true;
          setUnitOfMeasure(event.currentTarget.value);
        }}
      />
      <ContributeFields allowProductOptIn={!upstream} onChange={setShare} />
      {share.contribute && (
        <TextInput
          label="Barcode"
          description="Digits printed on the package. Required only when contributing this product."
          value={barcode}
          onChange={(event) => {
            draftDirty.current = true;
            setBarcode(event.currentTarget.value);
          }}
        />
      )}
      <Button size="xs" disabled={name.trim() === ''} loading={saving} onClick={() => void save()}>
        Save product
      </Button>
      {error !== '' && (
        <Alert color="red" py="xs">
          {error}
        </Alert>
      )}
    </Stack>
  );
};
