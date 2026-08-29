import { useState } from 'react';
import { Button } from '@mantine/core';
import { notifications } from '@mantine/notifications';
import { exportShoppingList } from '../../api/client';

export interface CartExportButtonProps {
  disabled?: boolean;
}

const exportErrorMessage = (error: unknown) =>
  error instanceof Error ? error.message : 'The shopping list could not be exported.';

export const CartExportButton = ({ disabled = false }: CartExportButtonProps) => {
  const [exporting, setExporting] = useState(false);

  const exportToCart = async () => {
    setExporting(true);
    try {
      const result = await exportShoppingList();
      if (result.failedItems !== undefined && result.failedItems.length > 0) {
        notifications.show({
          color: 'red',
          title: 'Cart export incomplete',
          message: `Could not export: ${result.failedItems.join(', ')}. These items remain on your shopping list.`,
        });
        return;
      }
      notifications.show({
        color: 'green',
        title: 'Shopping list exported',
        message: `${result.exported} item${result.exported === 1 ? '' : 's'} sent to your cart.`,
      });
    } catch (error) {
      notifications.show({
        color: 'red',
        title: 'Cart export incomplete',
        message: `${exportErrorMessage(error)} Failed items remain on your shopping list.`,
      });
    } finally {
      setExporting(false);
    }
  };

  return (
    <Button disabled={disabled} loading={exporting} onClick={() => void exportToCart()}>
      Export to cart
    </Button>
  );
};
