import { describe, expect, test } from 'bun:test'

import { renderToStaticMarkup } from 'react-dom/server'

import { FormField } from './form-field'

describe('FormField', () => {
  test('programmatically associates description and error text', () => {
    const markup = renderToStaticMarkup(
      <FormField
        description='Description'
        error='Error'
        htmlFor='field'
        label='Label'
      >
        <input aria-describedby='existing-help' id='field' />
      </FormField>
    )

    expect(markup).toContain(
      'aria-describedby="existing-help field-description field-error"'
    )
    expect(markup).toContain('aria-errormessage="field-error"')
    expect(markup).toContain('id="field-description"')
    expect(markup).toContain('id="field-error"')
  })
})
